package nmea0183

import (
	"bufio"
	"fmt"
	"log"
	"net"

	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk/mapper"
	"go.nanomsg.org/mangos/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	bufferSize int = 1024 // one NMEA message can by up to 82 bytes
)

// TCPConfig has all the required configuration for a TCPCollector
type TCPConfig struct {
	Host string
	Port uint16
}

// TCPCollector collects NMEA from a tcp server
type TCPCollector struct {
	Config TCPConfig
	Name   string
}

// NewTCPCollector creates an instance of a TCP collector
func NewTCPCollector(host string, port uint16, name string) *TCPCollector {
	return &TCPCollector{
		Config: TCPConfig{
			Host: host,
			Port: port,
		},
		Name: name,
	}
}

// Collect start the collection process and keeps running as long as there is data available
func (c *TCPCollector) Collect(socket mangos.Socket) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.Config.Host, c.Config.Port))
	if err != nil {
		return err
	}
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		m := &nanomsg.RawData{
			Header: &nanomsg.Header{
				HeaderSegments: []string{"collector", mapper.NMEA0183Type, c.Name},
			},
			Timestamp: timestamppb.Now(),
			Payload:   scanner.Bytes(),
		}
		toSend, err := proto.Marshal(m)
		if err != nil {
			log.Fatal(err)
		}
		if err := socket.Send(toSend); err != nil {
			log.Fatal(err)
		}
	}
	return nil
}
