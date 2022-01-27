package collector

import (
	"bufio"
	"net/url"
	"os"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/tarm/serial"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type SygoFileCollector struct {
	Path   string
	Config config.SygoConfig
}

// NewSygoFileCollector creates an instance of a file collector
func NewSygoFileCollector(url *url.URL, cfg config.SygoConfig) *SygoFileCollector {
	return &SygoFileCollector{
		Path:   url.Path,
		Config: cfg,
	}
}

// Collect start the collection process and keeps running as long as there is data available
func (c *SygoFileCollector) Collect(socket mangos.Socket) {
	stream := make(chan []byte, 1)

	go c.receive(stream)
	processStream(stream, config.SygoType, socket, c.Config.Name)
}

func (c *SygoFileCollector) receive(stream chan<- []byte) error {
	defer close(stream)

	fi, err := os.Stat(c.Path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeCharDevice == os.ModeCharDevice {
		s, err := serial.OpenPort(&serial.Config{Name: c.Path, Baud: c.Config.Baudrate})
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(s)
		for scanner.Scan() {
			x := scanner.Text() + "\n"
			logger.GetLogger().Warn(
				"Scanner got data",
				zap.String("String", x),
				zap.ByteString("Bytes", []byte(x)),
			)
			stream <- []byte(x)
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	} else {
		file, err := os.Open(c.Path)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			stream <- scanner.Bytes()
		}
	}

	return nil
}
