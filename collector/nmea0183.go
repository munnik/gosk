package collector

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/tarm/serial"
	"go.uber.org/zap"

	"go.nanomsg.org/mangos/v3"
)

// NMEA0183NetworkCollector collects NMEA from a tcp server
type NMEA0183NetworkCollector struct {
	URL    *url.URL
	Config config.NMEA0183Config
}

// NMEA0183FileCollector collects NMEA from a line based text file
type NMEA0183FileCollector struct {
	Path   string
	Config config.NMEA0183Config
}

// NewNMEA0183FileCollector creates an instance of a file collector
func NewNMEA0183FileCollector(url *url.URL, cfg config.NMEA0183Config) *NMEA0183FileCollector {
	return &NMEA0183FileCollector{
		Path:   url.Path,
		Config: cfg,
	}
}

// Collect start the collection process and keeps running as long as there is data available
func (c *NMEA0183FileCollector) Collect(socket mangos.Socket) {
	stream := make(chan []byte, 1)

	go c.receive(stream)
	processStream(stream, config.NMEA0183Type, socket, c.Config.Name)
}

func (c *NMEA0183FileCollector) receive(stream chan<- []byte) error {
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

// NewNMEA0183NetworkCollector creates an instance of a TCP collector
func NewNMEA0183NetworkCollector(url *url.URL, cfg config.NMEA0183Config) *NMEA0183NetworkCollector {
	return &NMEA0183NetworkCollector{
		URL:    url,
		Config: cfg,
	}
}

// Collect start the collection process and keeps running as long as there is data available
func (c *NMEA0183NetworkCollector) Collect(socket mangos.Socket) {
	stream := make(chan []byte, 1)

	go c.receive(stream)
	processStream(stream, config.NMEA0183Type, socket, c.Config.Name)
}

func (c *NMEA0183NetworkCollector) receive(stream chan<- []byte) error {
	defer close(stream)

	logger.GetLogger().Info(
		"Start to collect NMEA0183 data from the network",
		zap.String("Host", c.URL.Hostname()),
		zap.String("Port", c.URL.Port()),
	)
	if c.Config.Listen {
		if c.URL.Scheme == "tcp" {
			for {
				listener, err := net.Listen("tcp", fmt.Sprintf("%s:%s", c.URL.Hostname(), c.URL.Port()))
				if err != nil {
					logger.GetLogger().Warn(
						"Unable to start listening, sleeping for 5 seconds",
						zap.String("Host", c.URL.Hostname()),
						zap.String("Port", c.URL.Port()),
						zap.String("Error", err.Error()),
					)
					time.Sleep(5 * time.Second)
					continue
				}
				for {
					conn, err := listener.Accept()
					if err != nil {
						logger.GetLogger().Warn(
							"Unable to accept connection",
							zap.String("Host", c.URL.Hostname()),
							zap.String("Port", c.URL.Port()),
							zap.String("Error", err.Error()),
							zap.Any("Listener", listener),
						)
						continue
					}
					go handleConnection(conn, stream)
				}
			}
		} else if c.URL.Scheme == "udp" {
			for {
				conn, err := net.ListenPacket("udp", fmt.Sprintf("%s:%s", c.URL.Hostname(), c.URL.Port()))
				if err != nil {
					logger.GetLogger().Warn(
						"Unable to start listening, sleeping for 5 seconds",
						zap.String("Host", c.URL.Hostname()),
						zap.String("Port", c.URL.Port()),
						zap.String("Error", err.Error()),
					)
					time.Sleep(5 * time.Second)
					continue
				}
				buffer := make([]byte, 65507)
				for {
					size, _, err := conn.ReadFrom(buffer)
					if err != nil {
						logger.GetLogger().Warn(
							"Unable to read from connection",
							zap.String("Host", c.URL.Hostname()),
							zap.String("Port", c.URL.Port()),
							zap.String("Error", err.Error()),
							zap.Any("Connection", conn),
						)
						continue
					}
					stream <- buffer[:size]
				}
			}
		}
	} else {
		if c.URL.Scheme == "udp" || c.URL.Scheme == "tcp" {
			for {
				conn, err := net.Dial(c.URL.Scheme, fmt.Sprintf("%s:%s", c.URL.Hostname(), c.URL.Port()))
				if err != nil {
					logger.GetLogger().Warn(
						"Unable to make connection, sleeping for 5 seconds",
						zap.String("Host", c.URL.Hostname()),
						zap.String("Port", c.URL.Port()),
						zap.String("Error", err.Error()),
					)
					time.Sleep(5 * time.Second)
					continue
				}
				logger.GetLogger().Info(
					"Connection established, starting to read",
					zap.String("Host", c.URL.Hostname()),
					zap.String("Port", c.URL.Port()),
					zap.Any("Connection", conn),
				)
				handleConnection(conn, stream)
			}
		}
	}
	return nil
}
