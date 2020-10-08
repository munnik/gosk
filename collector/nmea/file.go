package nmea

import (
	"io"
	"io/ioutil"
	"log"
	"strings"
	"time"
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
func (collector FileCollector) Collect(writer io.Writer) error {
	data, err := ioutil.ReadFile(collector.Config.Path)
	if err != nil {
		return err
	}
	defer time.Sleep(time.Second) // wait for messages to flush before exiting the function

	lines := strings.Split(string(data[:]), "\n")
	lineCount := 0
	for _, line := range lines {
		log.Println("Sending new message with NNG", line)
		if _, err := writer.Write([]byte(line)); err != nil {
			return err
		}
		if lineCount++; lineCount == collector.Config.LinesAtOnce {
			time.Sleep(collector.Config.Interval)
			lineCount = 0
		}
	}
	return nil
}
