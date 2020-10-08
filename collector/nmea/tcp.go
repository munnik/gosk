package nmea

import (
	"bytes"
	"fmt"
	"io"
	"net"
)

const (
	bufferSize int = 1024 // one NMEA message can by up to 82 bytes
)

// TCPConfig has all the required configuration for a TCPCollector
type TCPConfig struct {
	Host string
	Port int
}

// TCPCollector collects NMEA from a tcp server
type TCPCollector struct {
	Config TCPConfig
}

// Collect start the collection process and keeps running as long as there is data available
func (collector TCPCollector) Collect(writer io.Writer) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", collector.Config.Host, collector.Config.Port))
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
				if _, err := writer.Write(append(lastLine[:lastLineLength], line...)); err != nil {
					return err
				}
			} else if index != lastLineIndex {
				if _, err := writer.Write(line); err != nil {
					return err
				}
			} else {
				copy(lastLine, line)
				lastLineLength = len(line)
			}
		}
	}
}
