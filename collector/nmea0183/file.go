package nmea0183

import (
	"io/ioutil"
	"strings"
	"time"

	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk/mapper"
	"go.nanomsg.org/mangos/v3"
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
func NewFileCollector(path string, interval time.Duration, linesAtOnce uint16, name string) *FileCollector {
	return &FileCollector{
		Config: FileConfig{
			Path:        path,
			Interval:    interval,
			LinesAtOnce: linesAtOnce,
		},
		Name: name,
	}
}

// Collect start the collection process and keeps running as long as there is data available
func (c *FileCollector) Collect(socket mangos.Socket) error {
	data, err := ioutil.ReadFile(c.Config.Path)
	if err != nil {
		return err
	}
	defer time.Sleep(time.Second) // wait for messages to flush before exiting the function

	lines := strings.Split(string(data), "\n")
	var lineCount uint16
	for _, line := range lines {
		m := nanomsg.NewMessage([]byte(line), time.Now(), []byte("collector"), []byte(mapper.NMEA0183Type), []byte(c.Name))
		if err := socket.Send([]byte(m.String())); err != nil {
			return err
		}
		if lineCount++; lineCount == c.Config.LinesAtOnce {
			time.Sleep(c.Config.Interval)
			lineCount = 0
		}
	}
	return nil
}
