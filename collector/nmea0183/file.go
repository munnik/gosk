package nmea0183

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/munnik/gosk/collector"
	"github.com/munnik/gosk/signalk/mapper"
)

// FileConfig has all the required configuration for a FileCollector
type FileConfig struct {
	Path        string
	Interval    time.Duration
	LinesAtOnce uint16
}

// FileCollector collects NMEA from a line based text file
type FileCollector struct {
	Config FileConfig
	Name   string
}

// NewFileCollector creates an instance of a file collector
func NewFileCollector(path string, interval time.Duration, linesAtOnce uint16, name string) collector.Collector {
	return FileCollector{
		Config: FileConfig{
			Path:        path,
			Interval:    interval,
			LinesAtOnce: linesAtOnce,
		},
		Name: name,
	}
}

// Collect start the collection process and keeps running as long as there is data available
func (c FileCollector) Collect(writer io.Writer) error {
	data, err := ioutil.ReadFile(c.Config.Path)
	if err != nil {
		return err
	}
	defer time.Sleep(time.Second) // wait for messages to flush before exiting the function

	lines := strings.Split(string(data[:]), "\n")
	var lineCount uint16
	msgPrefix := fmt.Sprintf(collector.Topic, mapper.NMEA0183Type, c.Name)
	for _, line := range lines {
		if _, err := writer.Write(append([]byte(msgPrefix), line...)); err != nil {
			return err
		}
		if lineCount++; lineCount == c.Config.LinesAtOnce {
			time.Sleep(c.Config.Interval)
			lineCount = 0
		}
	}
	return nil
}
