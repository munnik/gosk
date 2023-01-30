package reader

import (
	"encoding/json"
	"sync"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	mqttTopic = "vessels/#"
)

type MqttReader struct {
	mqttConfig *config.MQTTConfig
	publisher  mangos.Socket
	decoder    *zstd.Decoder
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

	m := mqtt.New(r.mqttConfig, r.messageReceived, mqttTopic)
	defer m.Disconnect()

	// never exit
	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait()
}

func (r *MqttReader) messageReceived(c paho.Client, m paho.Message) {
	received, err := r.decoder.DecodeAll(m.Payload(), nil)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not decompress payload",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}

	var deltas []message.Mapped
	if err := json.Unmarshal(received, &deltas); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", received),
		)
		return
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
