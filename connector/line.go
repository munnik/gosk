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
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

// LineConnector reads lines from the connection and sends it on the mangos socket
type LineConnector struct {
	config *config.ConnectorConfig
}

func NewLineConnector(c *config.ConnectorConfig) (*LineConnector, error) {
	return &LineConnector{config: c}, nil
}

func (r *LineConnector) Publish(publisher mangos.Socket) {
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

func (*LineConnector) AddSubscriber(subscriber mangos.Socket) {
	// do nothing
}

func (l *LineConnector) receive(stream chan<- []byte) error {
	reader, err := l.createReader()
	if err != nil {
		return err
	}
	return l.scan(reader, stream)
}

func (l LineConnector) createReader() (io.Reader, error) {
	var reader io.Reader
	var err error
	for {
		if l.config.URL.Scheme == "tcp" || l.config.URL.Scheme == "udp" {
			reader, err = l.createNetworkReader()
			if err == nil {
				break
			}
			logger.GetLogger().Warn(
				"Unable to create a reader, retrying in 5 seconds",
				zap.String("URL", l.config.URL.String()),
				zap.String("Error", err.Error()),
			)
			time.Sleep(5 * time.Second)
		} else if l.config.URL.Scheme == "file" {
			reader, err = l.createFileReader()
			if err == nil {
				break
			}
			logger.GetLogger().Warn(
				"Unable to create a reader, retrying in 5 seconds",
				zap.String("URL", l.config.URL.String()),
				zap.String("Error", err.Error()),
			)
			time.Sleep(5 * time.Second)
		} else {
			return nil, fmt.Errorf("unsupported connection scheme %v", l.config.URL.Scheme)
		}
	}
	return reader, nil
}

func (l LineConnector) createNetworkReader() (io.Reader, error) {
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
			return UdpListenerReader{conn: conn}, nil
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

func (l LineConnector) createFileReader() (io.Reader, error) {
	fi, err := os.Stat(l.config.URL.Path)
	if err != nil {
		return nil, fmt.Errorf("unable to stat the file %v, the error that occurred was %v", l.config.URL.Path, err)
	}
	var reader io.Reader
	if fi.Mode()&os.ModeCharDevice == os.ModeCharDevice {
		// the file is a serial device
		reader, err = serial.Open(&serial.Config{
			Address:  l.config.URL.Path,
			BaudRate: l.config.BaudRate,
			DataBits: l.config.DataBits,
			StopBits: l.config.StopBits,
			Parity:   string(config.ParityMap[l.config.Parity]),
		})
		if err != nil {
			return nil, fmt.Errorf("unable to open the port %v for reading, the error that occurred was %v", l.config.URL.Path, err)
		}
	} else {
		reader, err = os.Open(l.config.URL.Path)
		if err != nil {
			return nil, fmt.Errorf("unable to open the file %v for reading, the error that occurred was %v", l.config.URL.Path, err)
		}
	}
	return reader, nil
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

// UdpListenerReader implements the io.Reader interface
type UdpListenerReader struct {
	conn net.PacketConn
}

func (u UdpListenerReader) Read(p []byte) (n int, err error) {
	size, _, err := u.conn.ReadFrom(p)
	return size, err
}
