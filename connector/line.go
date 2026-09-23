package connector

import (
	"bufio"
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
	"go.bug.st/serial"
	"go.uber.org/zap"
)

// LineConnector reads lines from the connection and sends it on the mangos socket
type LineConnector struct {
	config *config.ConnectorConfig

	lock       sync.Mutex
	connection io.ReadWriter
}

// NewLineConnector checks that the url is one this connector can handle, but
// deliberately does not open it.
//
// Opening it here used to be what made a sensor that is not there stall a
// deploy. createConnection retries every 5 seconds until it succeeds, so with
// nothing on the other end the constructor never returned, Publish was never
// reached, and process - the only thing that ever publishes anything - never
// ran. These units are Type=notify and gosk signals readiness from its first
// published message (see nanomsg.Publisher.send), so the unit sat in
// "activating" for as long as the process lived. systemd keeps the start job
// open, switch-to-configuration waits on that job, and the whole activation
// stops: on node-rct-keizersgracht that held the system profile lock for 28
// hours and failed every deploy to that vessel, and it timed out the
// confirmation on node-rct-westlandgracht and node-shipit-maasdam.
//
// The connection is opened from receive instead, which runs in the goroutine
// Publish starts, so process is already running and reports
// DisconnectedOrNoData - accurate, and enough to make the unit ready - while
// the sensor is still missing.
func NewLineConnector(c *config.ConnectorConfig) (*LineConnector, error) {
	switch c.URL.Scheme {
	case "tcp", "udp", "file":
	default:
		return nil, fmt.Errorf("unsupported connection scheme %v", c.URL.Scheme)
	}
	return &LineConnector{config: c}, nil
}

func (r *LineConnector) Publish(publisher *nanomsg.Publisher[message.Raw]) {
	stream := make(chan []byte, 1)
	defer close(stream)
	go func() {
		for {
			if err := r.receive(stream); err != nil {
				logger.GetLogger().Warn(
					"Error while receiving data for the stream",
					zap.String("URL", r.config.URL.String()),
					zap.String("Error", err.Error()),
				)
			}
		}
	}()
	process(stream, r.config.Name, r.config.Protocol, publisher, r.config.Timeout)
}

func (r *LineConnector) Subscribe(subscriber *nanomsg.Subscriber[message.Raw]) {
	go func() {
		receiveBuffer := make(chan *message.Raw, bufferCapacity)
		go subscriber.Receive(receiveBuffer)

		for raw := range receiveBuffer {
			connection := r.currentConnection()
			if connection == nil {
				// Nothing is connected yet, or the connection just died and
				// receive is redialling. Dropping the write beats panicking
				// on a nil connection, which is what this did before the
				// connection became something that comes and goes.
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

func (l *LineConnector) receive(stream chan<- []byte) error {
	connection := l.createConnection()
	l.setConnection(connection)
	// Whatever ends the scan ends this connection with it, so the next call
	// dials a fresh one rather than scanning a socket the peer has gone away
	// from.
	defer l.setConnection(nil)
	return l.scan(connection, stream)
}

func (l *LineConnector) setConnection(connection io.ReadWriter) {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.connection = connection
}

func (l *LineConnector) currentConnection() io.ReadWriter {
	l.lock.Lock()
	defer l.lock.Unlock()
	return l.connection
}

// createConnection returns a connection, retrying until it has one. The url
// scheme is checked by NewLineConnector, so the only way this fails is the
// sensor not being there, which is not a reason to give up on it.
//
// It blocks, and is called from Publish's goroutine rather than from the
// constructor for exactly that reason - see NewLineConnector.
func (l *LineConnector) createConnection() io.ReadWriter {
	var lastError string
	for {
		var connection io.ReadWriter
		var err error
		if l.config.URL.Scheme == "file" {
			connection, err = l.createFileConnection()
		} else {
			connection, err = l.createNetworkConnection()
		}
		if err == nil {
			return connection
		}
		// A sensor that is absent fails identically every 5 seconds, so only
		// say so when the reason changes; the repeats go to debug.
		if err.Error() == lastError {
			logger.GetLogger().Debug(
				"Unable to create a connection, retrying in 5 seconds",
				zap.String("URL", l.config.URL.String()),
				zap.String("Error", err.Error()),
			)
		} else {
			logger.GetLogger().Warn(
				"Unable to create a connection, retrying in 5 seconds",
				zap.String("URL", l.config.URL.String()),
				zap.String("Error", err.Error()),
			)
			lastError = err.Error()
		}
		time.Sleep(5 * time.Second)
	}
}

func (l *LineConnector) createNetworkConnection() (io.ReadWriter, error) {
	if l.config.Listen {
		if l.config.URL.Scheme == "tcp" {
			listener, err := net.Listen(l.config.URL.Scheme, net.JoinHostPort(l.config.URL.Hostname(), l.config.URL.Port()))
			if err != nil {
				return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", l.config.URL.String(), err)
			}
			conn, err := listener.Accept()
			if err != nil {
				return nil, fmt.Errorf("unable to accept a connection on %v, the error that occurred was %v", l.config.URL.String(), err)
			}
			return conn, nil
		} else if l.config.URL.Scheme == "udp" {
			conn, err := net.ListenPacket(l.config.URL.Scheme, net.JoinHostPort(l.config.URL.Hostname(), l.config.URL.Port()))
			if err != nil {
				return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", l.config.URL.String(), err)
			}
			// TODO: test
			return UdpListenerConnection{conn: conn}, nil
		}
	} else {
		conn, err := net.Dial(l.config.URL.Scheme, net.JoinHostPort(l.config.URL.Hostname(), l.config.URL.Port()))
		if err != nil {
			return nil, fmt.Errorf("unable to dial to %v, the error that occurred was %v", l.config.URL.String(), err)
		}
		return conn, nil
	}
	return nil, nil
}

func (l *LineConnector) createFileConnection() (io.ReadWriter, error) {
	fi, err := os.Stat(l.config.URL.Path)
	if err != nil {
		return nil, fmt.Errorf("unable to stat the file %v, the error that occurred was %v", l.config.URL.Path, err)
	}
	var connection io.ReadWriter
	if fi.Mode()&os.ModeCharDevice == os.ModeCharDevice {
		mode := &serial.Mode{
			BaudRate: l.config.BaudRate,
			DataBits: l.config.DataBits,
		}
		switch l.config.StopBits {
		case "1":
			mode.StopBits = serial.OneStopBit
		case "1.5":
			mode.StopBits = serial.OnePointFiveStopBits
		case "2":
			mode.StopBits = serial.TwoStopBits
		default:
			return nil, fmt.Errorf("unsupport stop bits: %s", l.config.StopBits)
		}
		switch l.config.Parity {
		case "N":
			mode.Parity = serial.NoParity
		case "O":
			mode.Parity = serial.OddParity
		case "E":
			mode.Parity = serial.EvenParity
		default:
			return nil, fmt.Errorf("unsupport parity: %s", l.config.Parity)
		}
		// the file is a serial device
		connection, err = serial.Open(l.config.URL.Path, mode)
		if err != nil {
			return nil, fmt.Errorf("unable to open the port %v for reading and writing, the error that occurred was %v", l.config.URL.Path, err)
		}
		if port, ok := connection.(serial.Port); ok {
			port.SetReadTimeout(time.Second)
		}
	} else {
		connection, err = os.Open(l.config.URL.Path)
		if err != nil {
			return nil, fmt.Errorf("unable to open the file %v for reading and writing, the error that occurred was %v", l.config.URL.Path, err)
		}
	}
	return connection, nil
}

func (l *LineConnector) scan(reader io.Reader, stream chan<- []byte) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		// scanner.Bytes() aliases the scanner's own internal buffer, which
		// the next Scan() overwrites. Everything downstream of this channel
		// outlives that: process wraps the slice in a message.Raw (which
		// keeps it by reference), buffers it up to bufferCapacity deep, and
		// nanomsg.Publisher.Send marshals it later still. Handing the live
		// buffer over therefore doesn't just race, it silently publishes
		// whatever line happened to be scanned by the time the marshaller
		// got to it - in a reproduction of this pipeline, over half the
		// lines came out as the content of a later one. Copy per line.
		line := make([]byte, len(scanner.Bytes()))
		copy(line, scanner.Bytes())
		stream <- line
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error while scanning %v, the error that occurred was %v", l.config.URL.String(), err)
	}
	return nil
}

// UdpListenerConnection implements the io.ReadWriter interface
type UdpListenerConnection struct {
	conn net.PacketConn
}

func (u UdpListenerConnection) Read(p []byte) (n int, err error) {
	n, _, err = u.conn.ReadFrom(p)
	return
}

func (u UdpListenerConnection) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("could not write to UDP")
}
