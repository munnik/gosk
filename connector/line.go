package connector

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/goburrow/serial"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

// LineConnector reads lines from the connection and sends it on the mangos socket
type LineConnector struct {
	config     *config.ConnectorConfig
	connection io.ReadWriter
}

func NewLineConnector(c *config.ConnectorConfig) (*LineConnector, error) {
	var err error
	l := &LineConnector{config: c}
	l.connection, err = l.createConnection()
	if err != nil {
		return nil, err
	}
	return l, nil
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
	process(stream, r.config.Name, r.config.Protocol, publisher)
}

func (r *LineConnector) Subscribe(subscriber *nanomsg.Subscriber[message.Raw]) {
	go func() {
		receiveBuffer := make(chan *message.Raw, bufferCapacity)
		defer close(receiveBuffer)
		go subscriber.Receive(receiveBuffer)

		for raw := range receiveBuffer {
			r.connection.Write(append(raw.Value, '\r', '\n'))
		}
	}()
}

func (l *LineConnector) receive(stream chan<- []byte) error {
	return l.scan(l.connection, stream)
}

func (l LineConnector) createConnection() (io.ReadWriter, error) {
	var connection io.ReadWriter
	var err error
	for {
		if l.config.URL.Scheme == "tcp" || l.config.URL.Scheme == "udp" {
			connection, err = l.createNetworkConnection()
			if err == nil {
				break
			}
			logger.GetLogger().Warn(
				"Unable to create a connection, retrying in 5 seconds",
				zap.String("URL", l.config.URL.String()),
				zap.String("Error", err.Error()),
			)
			time.Sleep(5 * time.Second)
		} else if l.config.URL.Scheme == "file" {
			connection, err = l.createFileConnection()
			if err == nil {
				break
			}
			logger.GetLogger().Warn(
				"Unable to create a connection, retrying in 5 seconds",
				zap.String("URL", l.config.URL.String()),
				zap.String("Error", err.Error()),
			)
			time.Sleep(5 * time.Second)
		} else {
			return nil, fmt.Errorf("unsupported connection scheme %v", l.config.URL.Scheme)
		}
	}
	return connection, nil
}

func (l LineConnector) createNetworkConnection() (io.ReadWriter, error) {
	if l.config.Listen {
		if l.config.URL.Scheme == "tcp" {
			listener, err := net.Listen(l.config.URL.Scheme, fmt.Sprintf("%s:%s", l.config.URL.Hostname(), l.config.URL.Port()))
			if err != nil {
				return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", l.config.URL.String(), err)
			}
			conn, err := listener.Accept()
			if err != nil {
				return nil, fmt.Errorf("unable to accept a connection on %v, the error that occurred was %v", l.config.URL.String(), err)
			}
			return conn, nil
		} else if l.config.URL.Scheme == "udp" {
			conn, err := net.ListenPacket(l.config.URL.Scheme, fmt.Sprintf("%s:%s", l.config.URL.Hostname(), l.config.URL.Port()))
			if err != nil {
				return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", l.config.URL.String(), err)
			}
			// TODO: test
			return UdpListenerConnection{conn: conn}, nil
		}
	} else {
		conn, err := net.Dial(l.config.URL.Scheme, fmt.Sprintf("%s:%s", l.config.URL.Hostname(), l.config.URL.Port()))
		if err != nil {
			return nil, fmt.Errorf("unable to dial to %v, the error that occurred was %v", l.config.URL.String(), err)
		}
		return conn, nil
	}
	return nil, nil
}

func (l LineConnector) createFileConnection() (io.ReadWriter, error) {
	fi, err := os.Stat(l.config.URL.Path)
	if err != nil {
		return nil, fmt.Errorf("unable to stat the file %v, the error that occurred was %v", l.config.URL.Path, err)
	}
	var connection io.ReadWriter
	if fi.Mode()&os.ModeCharDevice == os.ModeCharDevice {
		// the file is a serial device
		connection, err = serial.Open(&serial.Config{
			Address:  l.config.URL.Path,
			BaudRate: l.config.BaudRate,
			DataBits: l.config.DataBits,
			StopBits: l.config.StopBits,
			Parity:   string(config.ParityMap[l.config.Parity]),
		})
		if err != nil {
			return nil, fmt.Errorf("unable to open the port %v for reading and writing, the error that occurred was %v", l.config.URL.Path, err)
		}
	} else {
		connection, err = os.Open(l.config.URL.Path)
		if err != nil {
			return nil, fmt.Errorf("unable to open the file %v for reading and writing, the error that occurred was %v", l.config.URL.Path, err)
		}
	}
	return connection, nil
}

func (l LineConnector) scan(reader io.Reader, stream chan<- []byte) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		stream <- scanner.Bytes()
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
	return 0, fmt.Errorf("Could not write to UDP")
}
