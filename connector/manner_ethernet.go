package connector

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

// MannerEthernetConnector reads from a socket and extracts the induvidual dataframes and sends it on the mangos socket
type MannerEthernetConnector struct {
	config     *config.ConnectorConfig
	connection io.ReadWriter
	timeout    *time.Timer
}

func NewMannerEthernetConnector(c *config.ConnectorConfig) (*MannerEthernetConnector, error) {
	var err error
	l := &MannerEthernetConnector{config: c, timeout: time.AfterFunc(c.Timeout, exit)}
	l.connection, err = l.createConnection()
	if err != nil {
		return nil, err
	}
	return l, nil
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
	process(stream, r.config.Name, r.config.Protocol, publisher, r.timeout, r.config.Timeout)
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
		defer close(receiveBuffer)
		go subscriber.Receive(receiveBuffer)

		for raw := range receiveBuffer {
			r.connection.Write(append(raw.Value, '\r', '\n'))
		}
	}()
}
func (r MannerEthernetConnector) readToChannel(streamBuffer chan byte) {
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, err := r.connection.Read(buffer)
			if err != nil {
				logger.GetLogger().Error("Error reading from the network stream", zap.Error(err))
			}
			if err == io.ErrUnexpectedEOF {
				os.Exit(0)
			}
			for i := 0; i < n; i++ {
				streamBuffer <- buffer[i]
			}
		}
	}()
}

func (r MannerEthernetConnector) createConnection() (io.ReadWriter, error) {
	var connection io.ReadWriter
	var err error
	for {
		if r.config.URL.Scheme == "tcp" || r.config.URL.Scheme == "udp" {
			connection, err = r.createNetworkConnection()
			if err == nil {
				break
			}
			logger.GetLogger().Warn(
				"Unable to create a connection, retrying in 5 seconds",
				zap.String("URL", r.config.URL.String()),
				zap.String("Error", err.Error()),
			)
			time.Sleep(5 * time.Second)
		} else {
			return nil, fmt.Errorf("unsupported connection scheme %v", r.config.URL.Scheme)
		}
	}
	return connection, nil
}

func (r MannerEthernetConnector) createNetworkConnection() (io.ReadWriter, error) {
	if r.config.Listen {
		if r.config.URL.Scheme == "tcp" {
			listener, err := net.Listen(r.config.URL.Scheme, fmt.Sprintf("%s:%s", r.config.URL.Hostname(), r.config.URL.Port()))
			if err != nil {
				return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", r.config.URL.String(), err)
			}
			conn, err := listener.Accept()
			if err != nil {
				return nil, fmt.Errorf("unable to accept a connection on %v, the error that occurred was %v", r.config.URL.String(), err)
			}
			return conn, nil
		} else if r.config.URL.Scheme == "udp" {
			conn, err := net.ListenPacket(r.config.URL.Scheme, fmt.Sprintf("%s:%s", r.config.URL.Hostname(), r.config.URL.Port()))
			if err != nil {
				return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", r.config.URL.String(), err)
			}
			// TODO: test
			return UdpListenerConnection{conn: conn}, nil
		}
	} else {
		conn, err := net.Dial(r.config.URL.Scheme, fmt.Sprintf("%s:%s", r.config.URL.Hostname(), r.config.URL.Port()))
		if err != nil {
			return nil, fmt.Errorf("unable to dial to %v, the error that occurred was %v", r.config.URL.String(), err)
		}
		return conn, nil
	}
	return nil, nil
}
