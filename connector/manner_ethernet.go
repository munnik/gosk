package connector

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

// MannerEthernetConnector reads from a socket and extracts the induvidual dataframes and sends it on the mangos socket
type MannerEthernetConnector struct {
	config *config.ConnectorConfig

	// listener is only used when config.Listen is set, and only ever from
	// the goroutine readToChannel starts - see listener.
	listener listener

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

// readToChannel feeds every byte the meter sends into streamBuffer,
// redialling whenever the connection ends.
//
// A read error used to exit the process, on the grounds that systemd
// restarting the unit was the only way to get a fresh connection, since
// createConnection ran once at construction. That made a meter that merely
// went away - unplugged, power-cycled, or a switch rebooting - look exactly
// like a crash: the unit flapped through restart after restart, and a
// deploy landing in one of those windows saw a unit that was not running.
// Reconnecting here instead keeps the process up, and process goes on
// reporting DisconnectedOrNoData for as long as the meter stays gone, which
// is what turns it into a notification rather than a restart loop.
func (r *MannerEthernetConnector) readToChannel(streamBuffer chan byte) {
	go func() {
		for {
			// Blocks until the meter answers. process is already running by
			// then, so the unit becomes ready - reporting
			// DisconnectedOrNoData - instead of hanging here; see
			// NewMannerEthernetConnector.
			connection := r.createConnection()
			r.setConnection(connection)
			r.read(connection, streamBuffer)
			// Whatever ended the read ended this connection with it, so
			// stop handing writes to it (see Subscribe) and let the next
			// pass dial a fresh one.
			r.setConnection(nil)
			if closer, ok := connection.(io.Closer); ok {
				closer.Close()
			}
			time.Sleep(retryConnectionInterval)
		}
	}()
}

// read copies bytes from connection into streamBuffer until the connection
// fails, and returns so its caller can dial a new one.
func (r *MannerEthernetConnector) read(connection io.ReadWriter, streamBuffer chan byte) {
	buffer := make([]byte, 1024)
	for {
		n, err := connection.Read(buffer)
		// Read may return data and an error together, so hand over what
		// did arrive before acting on the error.
		for i := 0; i < n; i++ {
			streamBuffer <- buffer[i]
		}
		// Any read error means this connection is finished, not just
		// io.ErrUnexpectedEOF: a TCP peer that goes away reports plain
		// io.EOF, which this used to log and then immediately retry on the
		// same dead connection, forever - a hot loop pinning a core and
		// flooding the journal.
		if err != nil {
			logger.GetLogger().Warn(
				"Error reading from the network stream, reconnecting",
				zap.String("URL", r.config.URL.String()),
				zap.Duration("Retrying in", retryConnectionInterval),
				zap.Error(err),
			)
			return
		}
	}
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
		switch r.config.URL.Scheme {
		case "tcp":
			// Accepted from a listener that stays bound for the life of
			// this connector, so the sensor can reconnect - see listener.
			return r.listener.accept(r.config.URL)
		case "udp":
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
