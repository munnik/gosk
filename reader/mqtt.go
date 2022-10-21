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
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	disconnectWait = 5000 // time to wait before disconnect in ms
	keepAlive      = 30 * time.Second
	readTopic      = "vessels/#"
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
	o.SetCleanSession(true) // TODO: verify
	o.SetUsername(r.config.Username)
	o.SetPassword(r.config.Password)
	o.SetOrderMatters(false)
	o.SetKeepAlive(keepAlive)
	o.SetDefaultPublishHandler(r.messageReceived)
	o.SetConnectionLostHandler(disconnectHandler)
	o.SetOnConnectHandler(r.connectHandler)

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

	// never exit
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
		nanomsg.SendMapped(&delta, r.publisher)
	}
}
func disconnectHandler(c mqtt.Client, e error) {
	if e != nil {
		logger.GetLogger().Warn(
			"MQTT connection lost",
			zap.String("Error", e.Error()),
		)
	}
}

func (r *MqttReader) connectHandler(c mqtt.Client) {
	logger.GetLogger().Info(
		"MQTT connection established",
	)

	if token := r.mqttClient.Subscribe(readTopic, 1, nil); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the MQTT topic",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", r.config.URLString),
		)
		return
	}
}
