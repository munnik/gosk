package connector

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/sdnotify"
	"go.uber.org/zap"
)

// warmupTimeoutExtension is how far ExtendTimeout pushes systemd's start
// timeout out on each call while readToChannel is still receiving bytes
// but Publish hasn't decoded (and published) a full 6-value frame yet -
// see readToChannel. Comfortably longer than sdnotify.ExtendTimeoutMinInterval
// so the extension never lapses between calls.
const warmupTimeoutExtension = 15 * time.Second

// MannerEthernetConnector reads from a socket and extracts the induvidual dataframes and sends it on the mangos socket
type MannerEthernetConnector struct {
	config *config.ConnectorConfig

	lock       sync.Mutex
	connection io.ReadWriter
}

// NewMannerEthernetConnector checks the url scheme but does not open the
// connection - readToChannel does, from the goroutine it starts.
//
// Opening it here stalled deploys whenever the meter was not reachable:
// createConnection retries every 5 seconds until it succeeds, so the
// constructor never returned, Publish never ran, and nothing was ever
// published - which is what a Type=notify gosk unit signals readiness from.
// The unit stayed in "activating", holding systemd's start job open and with
// it switch-to-configuration. See NewLineConnector, which had the same bug;
// between them they blocked node-rct-keizersgracht, node-rct-westlandgracht
// and node-shipit-maasdam.
func NewMannerEthernetConnector(c *config.ConnectorConfig) (*MannerEthernetConnector, error) {
	switch c.URL.Scheme {
	case "tcp", "udp":
	default:
		return nil, fmt.Errorf("unsupported connection scheme %v", c.URL.Scheme)
	}
	return &MannerEthernetConnector{config: c}, nil
}

func (r *MannerEthernetConnector) Publish(publisher *nanomsg.Publisher[message.Raw]) {
	stream := make(chan []byte, 1)
	defer close(stream)
	streamBuffer := make(chan byte, 4096)
	defer close(streamBuffer)
	r.readToChannel(streamBuffer)
	go func() {
		for b := range streamBuffer {
			if b&0b11000000 == 0b11000000 {
				values := make([]byte, 0, 12)
				values = binary.BigEndian.AppendUint16(values, uint16(extractFirstValue(b, streamBuffer)))
				for i := 1; i < 6; i++ {
					values = binary.BigEndian.AppendUint16(values, uint16(extractValue(streamBuffer)))
				}
				stream <- values
			}
		}
	}()
	process(stream, r.config.Name, r.config.Protocol, publisher, r.config.Timeout)
}

func extractValue(streamBuffer chan byte) int {
	byte1 := <-streamBuffer
	byte2 := <-streamBuffer
	byte3 := <-streamBuffer
	res := int(byte1&0b00111111)<<10 + int(byte2&0b00111111)<<4 + int(byte3&0b00111100)>>2
	return res
}
func extractFirstValue(byte1 byte, streamBuffer chan byte) int {
	byte2 := <-streamBuffer
	byte3 := <-streamBuffer
	res := int(byte1&0b00111111)<<10 + int(byte2&0b00111111)<<4 + int(byte3&0b00111100)>>2
	return res
}
func (r *MannerEthernetConnector) Subscribe(subscriber *nanomsg.Subscriber[message.Raw]) {
	go func() {
		receiveBuffer := make(chan *message.Raw, bufferCapacity)
		go subscriber.Receive(receiveBuffer)

		for raw := range receiveBuffer {
			connection := r.currentConnection()
			if connection == nil {
				logger.GetLogger().Warn(
					"Not connected, dropping the data to write",
					zap.String("URL", r.config.URL.String()),
				)
				continue
			}
			connection.Write(append(raw.Value, '\r', '\n'))
		}
	}()
}
func (r *MannerEthernetConnector) readToChannel(streamBuffer chan byte) {
	go func() {
		// Blocks until the meter answers. process is already running by
		// then, so the unit becomes ready - reporting DisconnectedOrNoData -
		// instead of hanging here; see NewMannerEthernetConnector.
		connection := r.createConnection()
		r.setConnection(connection)

		buffer := make([]byte, 1024)
		for {
			n, err := connection.Read(buffer)
			if err != nil {
				logger.GetLogger().Error("Error reading from the network stream", zap.Error(err))
			}
			// Any read error means this connection is finished, not just
			// io.ErrUnexpectedEOF: a TCP peer that goes away reports plain
			// io.EOF, which this used to log and then retry immediately,
			// forever - a hot loop pinning a core and flooding the journal
			// rather than reconnecting. Exiting lets systemd restart the
			// unit, which is how the connection gets rebuilt (createConnection
			// only runs at construction).
			if err != nil {
				os.Exit(0)
			}
			if n > 0 {
				// Publish only calls sdnotify.Ready() once a full 6-value
				// frame has been found and decoded (see the marker scan in
				// Publish) - on a cold start that can take a while to reach
				// if there's a backlog of data to scan through first, well
				// past systemd's Type=notify start timeout. Extend it for
				// as long as bytes are genuinely still arriving, so a
				// process making real progress doesn't get killed and
				// forced to reconnect and start scanning from zero again.
				sdnotify.ExtendTimeout(warmupTimeoutExtension)
			}
			for i := 0; i < n; i++ {
				streamBuffer <- buffer[i]
			}
		}
	}()
}

// createConnection returns a connection, retrying until it has one. The url
// scheme is checked by NewMannerEthernetConnector, so the only way this fails
// is the meter not being there, which is not a reason to give up on it.
func (r *MannerEthernetConnector) createConnection() io.ReadWriter {
	var lastError string
	for {
		connection, err := r.createNetworkConnection()
		if err == nil {
			return connection
		}
		// A meter that is absent fails identically every 5 seconds, so only
		// say so when the reason changes; the repeats go to debug.
		if err.Error() == lastError {
			logger.GetLogger().Debug(
				"Unable to create a connection, retrying in 5 seconds",
				zap.String("URL", r.config.URL.String()),
				zap.String("Error", err.Error()),
			)
		} else {
			logger.GetLogger().Warn(
				"Unable to create a connection, retrying in 5 seconds",
				zap.String("URL", r.config.URL.String()),
				zap.String("Error", err.Error()),
			)
			lastError = err.Error()
		}
		time.Sleep(5 * time.Second)
	}
}

func (r *MannerEthernetConnector) setConnection(connection io.ReadWriter) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.connection = connection
}

func (r *MannerEthernetConnector) currentConnection() io.ReadWriter {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.connection
}

func (r *MannerEthernetConnector) createNetworkConnection() (io.ReadWriter, error) {
	if r.config.Listen {
		if r.config.URL.Scheme == "tcp" {
			listener, err := net.Listen(r.config.URL.Scheme, net.JoinHostPort(r.config.URL.Hostname(), r.config.URL.Port()))
			if err != nil {
				return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", r.config.URL.String(), err)
			}
			conn, err := listener.Accept()
			if err != nil {
				return nil, fmt.Errorf("unable to accept a connection on %v, the error that occurred was %v", r.config.URL.String(), err)
			}
			return conn, nil
		} else if r.config.URL.Scheme == "udp" {
			conn, err := net.ListenPacket(r.config.URL.Scheme, net.JoinHostPort(r.config.URL.Hostname(), r.config.URL.Port()))
			if err != nil {
				return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", r.config.URL.String(), err)
			}
			// TODO: test
			return UdpListenerConnection{conn: conn}, nil
		}
	} else {
		conn, err := net.Dial(r.config.URL.Scheme, net.JoinHostPort(r.config.URL.Hostname(), r.config.URL.Port()))
		if err != nil {
			return nil, fmt.Errorf("unable to dial to %v, the error that occurred was %v", r.config.URL.String(), err)
		}
		return conn, nil
	}
	return nil, nil
}
