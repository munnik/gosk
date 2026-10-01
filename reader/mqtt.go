package reader

import (
	"encoding/json"
	"net/url"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/sdnotify"
)

const (
	mqttTopic      = "vessels/#"
	bufferCapacity = 5000
)

// connectedHeartbeatFactor makes watchSource re-announce a healthy source
// that many times less often than it repeats an unavailable one. Same
// reasoning, and deliberately the same value, as the connector package's
// constant of the same name: all the re-announcement has to do is bound how
// long a stale alarm can outlive a restart downstream, and the healthy case
// is the one that is true nearly all of the time.
const connectedHeartbeatFactor = 10

type MqttReader struct {
	mqttConfig   *config.MQTTConfig
	readerConfig *config.ReaderConfig
	sendBuffer   chan *message.Mapped
	// received carries one signal per message arriving from the broker, for
	// watchSource to notice that the source is alive. Buffered and written
	// to without blocking, because the writer is paho's callback: a reader
	// whose watchdog is momentarily busy must not stall message delivery,
	// and a signal dropped because one is already queued says exactly what
	// the queued one says.
	received                       chan struct{}
	decoder                        *zstd.Decoder
	mqttMessagesReceived           prometheus.Counter
	mqttMessagesDecompressed       prometheus.Counter
	mqttMessagesUnmarshalled       prometheus.Counter
	mqttTotalUpdatesSent           prometheus.Counter
	mqttTransferRequestUpdatesSent prometheus.Counter
}

func NewMqttReader(c *config.MQTTConfig, rc *config.ReaderConfig) *MqttReader {
	decoder, _ := zstd.NewReader(nil)

	// An unnamed reader would publish its notification under
	// "notifications.readers..connected", which names nothing and collides
	// with every other unnamed reader. The broker it reads from is the one
	// identifier it always has.
	if rc.Name == "" {
		rc.Name = brokerName(c.URLString)
	}

	return &MqttReader{
		mqttConfig:                     c,
		readerConfig:                   rc,
		received:                       make(chan struct{}, 1),
		decoder:                        decoder,
		mqttMessagesReceived:           promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_messages_received_total", Help: "total number of received mqtt messages"}),
		mqttMessagesDecompressed:       promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_messages_decompressed_total", Help: "total number of decompressed mqtt messages"}),
		mqttMessagesUnmarshalled:       promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_messages_unmarshalled_total", Help: "total number of unmarshalled mqtt messages"}),
		mqttTotalUpdatesSent:           promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_updates_sent_total", Help: "total number of updates sent"}),
		mqttTransferRequestUpdatesSent: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_updates_sent_transfer_request", Help: "number of updates sent via a transfer request"}),
	}
}

func (r *MqttReader) ReadMapped(publisher *nanomsg.Publisher[message.Mapped]) {
	r.sendBuffer = make(chan *message.Mapped, bufferCapacity)
	defer close(r.sendBuffer)
	go publisher.Send(r.sendBuffer)

	// Reaching here is what "started" means for this reader, so say so
	// before connecting rather than leaving it to nanomsg.Publisher's
	// first successful send (see pub.go's send). That send needs a vessel
	// to have actually published something on mqttTopic, and for it to
	// decompress, unmarshal and publish - none of which this process
	// controls - and mqtt.New below blocks for up to
	// mqtt.initialConnectWait first, so on a quiet fleet or a broker that
	// has just been restarted the unit could sit below gosk.nix's
	// TimeoutStartSec (40s) and be killed while doing nothing wrong. A
	// broker that is not up yet is not this process's health either; paho
	// retries it in the background and subscribes on every connection.
	sdnotify.Ready()

	go r.watchSource()

	m := mqtt.New(r.mqttConfig, r.messageHandler, mqttTopic)
	defer m.Disconnect()

	// never exit
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
}

// watchSource publishes a SignalK notification whenever this reader's source
// stops producing data - the broker is unreachable, or every vessel feeding
// it has gone quiet - and clears it when data returns. It is the reader's
// equivalent of what connector/main.go's process does for a sensor, and
// exists for the same reason: a source that is not there is something to
// raise an alarm about, not something for this process to exit or stay
// unstarted over.
//
// Unlike a connector, a reader has no mapping stage downstream to turn a
// status report into a notification - it publishes message.Mapped itself -
// so it builds the notification directly, see mapper.NewReaderStatusUpdate.
func (r *MqttReader) watchSource() {
	timeoutDuration := r.readerConfig.Timeout
	// time.NewTicker panics on a non-positive interval, so `timeout: 0` in a
	// config file would take the reader down at startup over the very
	// mechanism meant to keep it up. A zero timeout is meaningless anyway:
	// it would report the source missing continuously, however healthy it
	// is. The same guard connector/main.go's process applies.
	if timeoutDuration <= 0 {
		logger.GetLogger().Warn(
			"Timeout must be positive, using the default instead",
			zap.String("Reader", r.readerConfig.Name),
			zap.Duration("Configured", timeoutDuration),
			zap.Duration("Using", config.DefaultReaderTimeout),
		)
		timeoutDuration = config.DefaultReaderTimeout
	}

	timeout := time.NewTimer(timeoutDuration)
	defer timeout.Stop()
	heartbeat := time.NewTicker(connectedHeartbeatFactor * timeoutDuration)
	defer heartbeat.Stop()

	// Starts false, so a reader whose broker never answers raises the alarm
	// after one timeout rather than waiting to have been connected first.
	connected := false
	for {
		select {
		case <-r.received:
			timeout.Reset(timeoutDuration)
			if !connected {
				connected = true
				r.sendBuffer <- r.status(true)
			}
		case <-timeout.C:
			if connected {
				logger.GetLogger().Warn(
					"Timeout receiving data from the source, no data received",
					zap.String("Reader", r.readerConfig.Name),
					zap.String("URL", r.mqttConfig.URLString),
				)
			}
			connected = false
			r.sendBuffer <- r.status(false)
			timeout.Reset(timeoutDuration)
		case <-heartbeat.C:
			// Re-announce the healthy state so a stale alarm cannot outlive
			// a restart of whatever consumes this - nanomsg pub/sub has no
			// replay for a late subscriber, so the single clearing message
			// sent when data returned may have reached nobody. The timeout
			// above already repeats the unhealthy case.
			if connected {
				r.sendBuffer <- r.status(true)
			}
		}
	}
}

func (r *MqttReader) status(connected bool) *message.Mapped {
	return mapper.NewReaderStatusUpdate(r.readerConfig.Context, r.readerConfig.Name, connected)
}

// brokerName is the identifier a reader falls back to when none is
// configured: the broker's host, or the whole url when it cannot be parsed
// (it is only ever used to name the reader, so an odd-looking name beats no
// name).
func brokerName(urlString string) string {
	if u, err := url.Parse(urlString); err == nil && u.Host != "" {
		return u.Host
	}
	return urlString
}

func (r *MqttReader) messageHandler(c paho.Client, m paho.Message) {
	r.mqttMessagesReceived.Inc()
	// Anything arriving at all means the source is producing data, so signal
	// watchSource before the payload is decoded rather than after. A payload
	// this reader cannot decompress or unmarshal is a separate problem (and
	// logged as one below); it is not the source being unavailable, and
	// reporting it as such would hide the real fault behind the wrong alarm.
	// connector/main.go's process counts bytes off the wire the same way,
	// before any mapping.
	select {
	case r.received <- struct{}{}:
	default:
	}
	var received []byte
	var err error
	if r.mqttConfig.Compress {
		received, err = r.decoder.DecodeAll(m.Payload(), nil)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not decompress payload",
				zap.String("Error", err.Error()),
				zap.ByteString("Bytes", m.Payload()),
			)
			return
		}
		r.mqttMessagesDecompressed.Inc()
	} else {
		received = m.Payload()
	}

	messages := make([]*message.Mapped, 0)
	if err := json.Unmarshal(received, &messages); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", received),
		)
		return
	}
	r.mqttMessagesUnmarshalled.Inc()

	for _, message := range messages {
		r.sendBuffer <- message

		for _, update := range message.Updates {
			if update.Source.TransferUuid != uuid.Nil {
				r.mqttTransferRequestUpdatesSent.Inc()
			}
			r.mqttTotalUpdatesSent.Inc()
		}
	}
}
