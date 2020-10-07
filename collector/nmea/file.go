package nmea

import (
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/go-zeromq/zmq4"
)

// FileConfig has all the required configuration for a FileCollector
type FileConfig struct {
	Path        string
	Interval    time.Duration
	LinesAtOnce int
}

// FileCollector collects NMEA from a line based text file
type FileCollector struct {
	Config FileConfig
}

// Collect start the collection process and keeps running as long as there is data available
func (collector FileCollector) Collect(socket zmq4.Socket) error {
	data, err := ioutil.ReadFile(collector.Config.Path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data[:]), "\n")
	lineCount := 0
	for _, line := range lines {
		log.Println("Sending new message with ZMQ", line)
		socket.Send(zmq4.NewMsgFrom([]byte("NMEA"), []byte(line)))
		if lineCount++; lineCount == collector.Config.LinesAtOnce {
			time.Sleep(collector.Config.Interval)
			lineCount = 0
		}
	}
	return nil
}
