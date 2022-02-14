package collector

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

// LineReader reads lines from the connection and sends it on the mangos socket
type LineReader struct {
	config config.CollectorConfig
}

func NewLineReader(c config.CollectorConfig) (*LineReader, error) {
	return &LineReader{config: c}, nil
}

func (l *LineReader) Collect(publisher mangos.Socket) {
	stream := make(chan []byte, 1)
	go l.receive(stream)
	process(stream, l.config.Name, publisher)
}

func (l *LineReader) receive(stream chan<- []byte) error {
	defer close(stream)

	reader, err := l.createReader()
	if err != nil {
		return err
	}
	return l.scan(reader, stream)
}

func (l LineReader) createReader() (io.Reader, error) {
	var reader io.Reader
	var err error
	for {
		if l.config.URI.Scheme == "tcp" || l.config.URI.Scheme == "udp" {
			reader, err = l.createNetworkReader()
			if err == nil {
				break
			}
			logger.GetLogger().Warn(
				"Unable to create a reader, retrying in 5 seconds",
				zap.String("URI", l.config.URI.String()),
				zap.String("Error", err.Error()),
			)
			time.Sleep(5 * time.Second)
		} else if l.config.URI.Scheme == "file" {
			reader, err = l.createFileReader()
			if err == nil {
				break
			}
			logger.GetLogger().Warn(
				"Unable to create a reader, retrying in 5 seconds",
				zap.String("URI", l.config.URI.String()),
				zap.String("Error", err.Error()),
			)
			time.Sleep(5 * time.Second)
		} else {
			return nil, fmt.Errorf("unsupported connection scheme %v", l.config.URI.Scheme)
		}
	}
	return reader, nil
}

func (l LineReader) createNetworkReader() (io.Reader, error) {
	if l.config.Listen {
		if l.config.URI.Scheme == "tcp" {
			listener, err := net.Listen(l.config.URI.Scheme, fmt.Sprintf("%s:%s", l.config.URI.Hostname(), l.config.URI.Port()))
			if err != nil {
				return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", l.config.URI.String(), err)
			}
			conn, err := listener.Accept()
			if err != nil {
				return nil, fmt.Errorf("unable to accept a connection on %v, the error that occurred was %v", l.config.URI.String(), err)
			}
			return conn, nil
		} else if l.config.URI.Scheme == "udp" {
			conn, err := net.ListenPacket(l.config.URI.Scheme, fmt.Sprintf("%s:%s", l.config.URI.Hostname(), l.config.URI.Port()))
			if err != nil {
				return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", l.config.URI.String(), err)
			}
			// TODO: test
			return UdpListenerReader{conn: conn}, nil
		}
	} else {
		conn, err := net.Dial(l.config.URI.Scheme, fmt.Sprintf("%s:%s", l.config.URI.Hostname(), l.config.URI.Port()))
		if err != nil {
			return nil, fmt.Errorf("unable to dial to %v, the error that occurred was %v", l.config.URI.String(), err)
		}
		return conn, nil
	}
	return nil, nil
}

func (l LineReader) createFileReader() (io.Reader, error) {
	fi, err := os.Stat(l.config.URI.Path)
	if err != nil {
		return nil, fmt.Errorf("unable to stat the file %v, the error that occurred was %v", l.config.URI.Path, err)
	}
	var reader io.Reader
	if fi.Mode()&os.ModeCharDevice == os.ModeCharDevice {
		// the file is a serial device
		reader, err = serial.Open(&serial.Config{
			Address:  l.config.URI.Path,
			BaudRate: l.config.BaudRate,
			DataBits: l.config.DataBits,
			StopBits: l.config.StopBits,
			Parity:   string(config.ParityMap[l.config.Parity]),
		})
		if err != nil {
			return nil, fmt.Errorf("unable to open the port %v for reading, the error that occurred was %v", l.config.URI.Path, err)
		}
	} else {
		reader, err = os.Open(l.config.URI.Path)
		if err != nil {
			return nil, fmt.Errorf("unable to open the file %v for reading, the error that occurred was %v", l.config.URI.Path, err)
		}
	}
	return reader, nil
}

func (l LineReader) scan(reader io.Reader, stream chan<- []byte) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		stream <- scanner.Bytes()
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error while scanning %v, the error that occurred was %v", l.config.URI.String(), err)
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
