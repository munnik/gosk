package reader

import (
	"encoding/json"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"

	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"go.nanomsg.org/mangos/v3"
)

const (
	mqttTopic                    = "vessels/#"
	receivedBufferSize           = 10000 // allow a lot of compressed data
	numberOfDecompressionWorkers = 10
	decompressedBufferSize       = 1000
	numberOfPublishWorkers       = 200
)

type MqttReader struct {
	mqttConfig         *config.MQTTConfig
	publisher          mangos.Socket
	decoder            *zstd.Decoder
	receivedBuffer     chan []byte
	decompressedBuffer chan []byte
}

func NewMqttReader(c *config.MQTTConfig) *MqttReader {
	decoder, _ := zstd.NewReader(nil)
	return &MqttReader{
		mqttConfig: c,
		decoder:    decoder,
	}
}

func (r *MqttReader) ReadMapped(publisher mangos.Socket) {
	r.publisher = publisher

	r.receivedBuffer = make(chan []byte, receivedBufferSize)
	defer close(r.receivedBuffer)
	for i := 0; i < numberOfDecompressionWorkers; i++ {
		go r.decompressionWorker()
	}
	r.decompressedBuffer = make(chan []byte, decompressedBufferSize)
	defer close(r.decompressedBuffer)
	for i := 0; i < numberOfPublishWorkers; i++ {
		go r.publishWorker()
	}

	go r.reportBufferSizes()

	m := mqtt.New(r.mqttConfig, r.messageReceived, mqttTopic)
	defer m.Disconnect()

	// never exit
	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait()
}

func (r *MqttReader) messageReceived(c paho.Client, m paho.Message) {
	select {
	case r.receivedBuffer <- m.Payload():
	default:
		logger.GetLogger().Warn(
			"Could not add to the receivedBuffer, probably the buffer is full. Dropping message.",
			zap.Int("Buffer size", len(r.receivedBuffer)),
		)
	}
}

func (r *MqttReader) decompressionWorker() {
	for bytes := range r.receivedBuffer {
		decompressedBytes, err := r.decoder.DecodeAll(bytes, nil)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not decompress payload",
				zap.String("Error", err.Error()),
				zap.ByteString("Bytes", bytes),
			)
			continue
		}

		select {
		case r.decompressedBuffer <- decompressedBytes:
		default:
			logger.GetLogger().Warn(
				"Could not add to the decompressedBuffer, probably the buffer is full. Dropping message.",
				zap.Int("Buffer size", len(r.decompressedBuffer)),
			)
		}
	}
}

func (r *MqttReader) publishWorker() {
	for bytes := range r.decompressedBuffer {
		var deltas []message.Mapped
		if err := json.Unmarshal(bytes, &deltas); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal buffer",
				zap.String("Error", err.Error()),
				zap.ByteString("Bytes", bytes),
			)
			continue
		}

		for _, delta := range deltas {
			bytes, err := json.Marshal(delta)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not marshal delta",
					zap.String("Error", err.Error()),
				)
				continue
			}
			if err := r.publisher.Send(bytes); err != nil {
				logger.GetLogger().Warn(
					"Unable to send the message using NanoMSG",
					zap.ByteString("Message", bytes),
					zap.String("Error", err.Error()),
				)
				continue
			}
		}
	}
}

func (r *MqttReader) reportBufferSizes() {
	for {
		logger.GetLogger().Warn(
			"Current buffer sizes",
			zap.Int("Received buffer", len(r.receivedBuffer)),
			zap.Int("Decompressed buffer", len(r.decompressedBuffer)),
		)
		time.Sleep(10 * time.Second)
	}
}
