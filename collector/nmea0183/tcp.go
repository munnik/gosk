package nmea0183

import (
	"bytes"
	"fmt"
	"net"
	"time"

	"github.com/munnik/gosk/collector"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk/mapper"
	"go.nanomsg.org/mangos/v3"
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
func NewTCPCollector(host string, port uint16, name string) collector.Collector {
	return TCPCollector{
		Config: TCPConfig{
			Host: host,
			Port: port,
		},
		Name: name,
	}
}

// Collect start the collection process and keeps running as long as there is data available
func (c TCPCollector) Collect(socket mangos.Socket) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.Config.Host, c.Config.Port))
	if err != nil {
		return err
	}
	defer conn.Close()

	buffer := make([]byte, bufferSize)
	newLine := []byte{13, 10}
	// used to hold the last line if it didn't end in a newline
	lastLine := make([]byte, bufferSize)
	lastLineLength := 0
	for {
		if _, err := conn.Read(buffer); err != nil {
			return err
		}
		lines := bytes.Split(buffer, newLine)
		lastLineIndex := len(lines) - 1
		for index, line := range lines {
			if index == 0 {
				// prepend the lastLine to complete the line
				m := nanomsg.Create(append(lastLine[:lastLineLength], line...), time.Now(), []byte("collector"), []byte(mapper.NMEA0183Type), []byte(c.Name))
				if err := socket.Send([]byte(m.String())); err != nil {
					return err
				}
			} else if index != lastLineIndex {
				m := nanomsg.Create(line, time.Now(), []byte("collector"), []byte(mapper.NMEA0183Type), []byte(c.Name))
				if err := socket.Send([]byte(m.String())); err != nil {
					return err
				}
			} else {
				copy(lastLine, line)
				lastLineLength = len(line)
			}
		}
	}
}
