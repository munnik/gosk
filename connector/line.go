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

func (r *LineConnector) Connect(publisher mangos.Socket, subscriber mangos.Socket) {
	readWriter, err := r.createReadWriter()
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to create a readWriter",
			zap.String("Error", err.Error()),
		)
		return
	}

	// write data to the connection
	go func() {
		for {
			received, err := subscriber.Recv()
			if err != nil {
				logger.GetLogger().Error(
					"Unable to receive data from the subscriber",
					zap.String("Error", err.Error()),
				)
				continue
			}
			readWriter.Write(received)
		}
	}()

	// receive data from the connection
	c := make(chan []byte, receiveChannelBufferSize)
	defer close(c)
	go func() {
		for {
			if err := r.scan(readWriter, c); err != nil {
				logger.GetLogger().Warn(
					"Error while receiving data for the stream",
					zap.String("URL", r.config.URL.String()),
					zap.String("Error", err.Error()),
				)
			}
		}
	}()
	process(c, r.config.Name, r.config.Protocol, publisher)
}

func (l LineConnector) createReadWriter() (io.ReadWriter, error) {
	var readWriter io.ReadWriter
	var err error
	for {
		if l.config.URL.Scheme == "tcp" || l.config.URL.Scheme == "udp" {
			readWriter, err = l.createNetworkReadWriter()
			if err == nil {
				return readWriter, nil
			}
			logger.GetLogger().Warn(
				"Unable to create a reader, retrying in 5 seconds",
				zap.String("URL", l.config.URL.String()),
				zap.String("Error", err.Error()),
			)
			time.Sleep(5 * time.Second)
		} else if l.config.URL.Scheme == "file" {
			readWriter, err = l.createFileReadWriter()
			if err == nil {
				return readWriter, nil

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
}

func (l LineConnector) createNetworkReadWriter() (io.ReadWriter, error) {
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
			return UdpListenerReadWriter{conn: conn}, nil
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

func (l LineConnector) createFileReadWriter() (io.ReadWriter, error) {
	fi, err := os.Stat(l.config.URL.Path)
	if err != nil {
		return nil, fmt.Errorf("unable to stat the file %v, the error that occurred was %v", l.config.URL.Path, err)
	}
	var readWriter io.ReadWriter
	if fi.Mode()&os.ModeCharDevice == os.ModeCharDevice {
		// the file is a serial device
		readWriter, err = serial.Open(&serial.Config{
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
		readWriter, err = os.Open(l.config.URL.Path)
		if err != nil {
			return nil, fmt.Errorf("unable to open the file %v for reading, the error that occurred was %v", l.config.URL.Path, err)
		}
	}
	return readWriter, nil
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

// UdpListenerReadWriter implements the io.ReadWriter interface
type UdpListenerReadWriter struct {
	conn net.PacketConn
}

func (u UdpListenerReadWriter) Read(p []byte) (n int, err error) {
	size, _, err := u.conn.ReadFrom(p)
	return size, err
}

func (u UdpListenerReadWriter) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("could not write connection is listen only")
}
