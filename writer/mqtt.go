package writer

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

const (
	disconnectWait = 5000 // time to wait before disconnect in ms
	keepAlive      = 30 * time.Second
	writeTopic     = "vessels/urn:mrn:imo:mmsi:%s"
)

type MqttWriter struct {
	mqttConfig     *config.MQTTConfig
	mqttClient     *mqtt.Client
	useA           bool
	bufferA        []*[]byte
	bufferB        []*[]byte
	bufferCapacity int
	lastFlush      time.Time
	encoder        *zstd.Encoder
	writeMutex     sync.Mutex
}

func NewMqttWriter(c *config.MQTTConfig) *MqttWriter {
	w := &MqttWriter{mqttConfig: c, useA: true}
	encoder, _ := zstd.NewWriter(nil)
	w.encoder = encoder
	w.bufferCapacity = int(math.Floor(1.1 * float64(c.BufferSize)))
	w.bufferA = make([]*[]byte, 0, w.bufferCapacity)
	w.bufferB = make([]*[]byte, 0, w.bufferCapacity)
	w.writeMutex = sync.Mutex{}
	w.lastFlush = time.Now()
	return w
}

func (w *MqttWriter) sendMQTT(deltas []message.Mapped) {
	bytes, err := json.Marshal(deltas)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshall the deltas",
			zap.String("Error", err.Error()),
		)
		return
	}
	go func(context string, bytes []byte) {
		compressed := w.encoder.EncodeAll(bytes, make([]byte, 0, len(bytes)))
		w.mqttClient.Publish(context, 0, true, compressed)
	}(fmt.Sprintf(writeTopic, w.mqttConfig.Username), bytes)
}

func (w *MqttWriter) WriteMapped(subscriber *nanomsg.Subscriber[message.Mapped]) {
	w.mqttClient = mqtt.New(w.mqttConfig, nil, "")
	defer w.mqttClient.Disconnect()
	receiveBuffer := make(chan *message.Mapped, bufferCapacity)
	defer close(receiveBuffer)
	go subscriber.Receive(receiveBuffer)

	for mapped := range receiveBuffer {
		bytes, err := json.Marshal(mapped)
		subscriber.ReturnToPool(mapped)
		if err != nil {
			logger.GetLogger().Warn("Unable to marshall mapped message")
			continue
		}
		w.appendToCache(&bytes)
	}
}

func (w *MqttWriter) appendToCache(received *[]byte) {
	w.writeMutex.Lock()
	if w.useA {
		w.bufferA = append(w.bufferA, received)
	} else {
		w.bufferB = append(w.bufferB, received)
	}
	w.writeMutex.Unlock()
	if w.lenCache() > w.mqttConfig.BufferSize || time.Since(w.lastFlush) > w.mqttConfig.Interval {
		w.flushCache()
	}
}

func (w *MqttWriter) lenCache() int {
	if w.useA {
		return len(w.bufferA)
	} else {
		return len(w.bufferB)
	}
}
func (w *MqttWriter) flushCache() {
	w.writeMutex.Lock()
	w.useA = !w.useA
	w.writeMutex.Unlock()
	w.lastFlush = time.Now()
	var buffer *[]*[]byte
	if !w.useA {
		buffer = &w.bufferA
	} else {
		buffer = &w.bufferB
	}
	deltas := make([]message.Mapped, 0, len(*buffer))
	for _, v := range *buffer {
		var m message.Mapped
		if err := json.Unmarshal(*v, &m); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal a message from the publisher",
				zap.String("Error", err.Error()),
			)
			return
		}
		deltas = append(deltas, m)
	}
	w.sendMQTT(deltas)

	if !w.useA {
		w.bufferA = make([]*[]byte, 0, w.bufferCapacity)
	} else {
		w.bufferB = make([]*[]byte, 0, w.bufferCapacity)
	}
}
