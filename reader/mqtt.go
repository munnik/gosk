package reader

import (
	"encoding/json"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	disconnectWait = 5000 // time to wait before disconnect in ms
	keepAlive      = 30 * time.Second
)

type MqttReader struct {
	config     *config.MQTTConfig
	mqttClient mqtt.Client
	publisher  mangos.Socket
	decoder    *zstd.Decoder
}

func NewMqttReader(c *config.MQTTConfig) *MqttReader {
	var decoder, _ = zstd.NewReader(nil)
	return &MqttReader{
		config:  c,
		decoder: decoder,
	}
}

func (r *MqttReader) createClientOptions() *mqtt.ClientOptions {
	o := mqtt.NewClientOptions()
	o.AddBroker(r.config.URLString)
	o.SetClientID(r.config.ClientId)
	o.SetCleanSession(true) // TODO: verify
	o.SetUsername(r.config.Username)
	o.SetPassword(r.config.Password)
	o.SetOrderMatters(false)
	o.SetKeepAlive(keepAlive)
	o.SetDefaultPublishHandler(r.messageReceived)
	return o
}

func (r *MqttReader) ReadMapped(publisher mangos.Socket) {
	r.publisher = publisher
	r.mqttClient = mqtt.NewClient(r.createClientOptions())
	if token := r.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the MQTT broker",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", r.config.URLString),
		)
		return
	}
	defer r.mqttClient.Disconnect(disconnectWait)

	if token := r.mqttClient.Subscribe(r.config.Context, 1, nil); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the MQTT topic",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", r.config.URLString),
		)
		return
	}
	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait()
}

func (r *MqttReader) messageReceived(c mqtt.Client, m mqtt.Message) {
	received, err := r.decoder.DecodeAll(m.Payload(), nil)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not decompress payload",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}

	b := message.NewBuffer()
	if err := json.Unmarshal(received, b); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", received),
		)
		return
	}

	for _, delta := range b.Deltas {
		bytes, err := json.Marshal(delta)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not marshal delta",
				zap.String("Error", err.Error()),
			)
			continue
		}
		r.publisher.Send(bytes)
	}
}
