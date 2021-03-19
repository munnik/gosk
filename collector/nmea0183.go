package collector

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper"
	"github.com/tarm/serial"
	"go.uber.org/zap"

	"go.nanomsg.org/mangos/v3"
)

// NMEA0183NetworkCollector collects NMEA from a tcp server
type NMEA0183NetworkCollector struct {
	URL  *url.URL
	Dial bool
	Name string
}

// NMEA0183FileCollector collects NMEA from a line based text file
type NMEA0183FileCollector struct {
	Path     string
	BaudRate int
	Name     string
}

// NewNMEA0183FileCollector creates an instance of a file collector
func NewNMEA0183FileCollector(url *url.URL, baudRate int, name string) *NMEA0183FileCollector {
	return &NMEA0183FileCollector{
		Path:     url.Path,
		BaudRate: baudRate,
		Name:     name,
	}
}

// Collect start the collection process and keeps running as long as there is data available
func (c *NMEA0183FileCollector) Collect(socket mangos.Socket) {
	stream := make(chan []byte, 1)

	go c.receive(stream)
	processStream(stream, mapper.NMEA0183Type, socket, c.Name)
}

func (c *NMEA0183FileCollector) receive(stream chan<- []byte) error {
	defer close(stream)

	fi, err := os.Stat(c.Path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeCharDevice == os.ModeCharDevice {
		s, err := serial.OpenPort(&serial.Config{Name: c.Path, Baud: c.BaudRate})
		if err != nil {
			return err
		}
		buffer := make([]byte, 1024)
		for {
			n, err := s.Read(buffer)
			if err != nil {
				return err
			}
			stream <- buffer[:n]
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
func NewNMEA0183NetworkCollector(url *url.URL, dial bool, name string) *NMEA0183NetworkCollector {
	return &NMEA0183NetworkCollector{
		URL:  url,
		Dial: dial,
		Name: name,
	}
}

// Collect start the collection process and keeps running as long as there is data available
func (c *NMEA0183NetworkCollector) Collect(socket mangos.Socket) {
	stream := make(chan []byte, 1)

	go c.receive(stream)
	processStream(stream, mapper.NMEA0183Type, socket, c.Name)
}

func (c *NMEA0183NetworkCollector) receive(stream chan<- []byte) error {
	defer close(stream)

	logger.GetLogger().Info(
		"Start to collect NMEA0183 data from the network",
		zap.String("Host", c.URL.Hostname()),
		zap.String("Port", c.URL.Port()),
	)
	if c.Dial {
		if c.URL.Scheme == "udp" || c.URL.Scheme == "tcp" {
			conn, err := net.Dial(c.URL.Scheme, fmt.Sprintf("%s:%s", c.URL.Hostname(), c.URL.Port()))
			if err != nil {
				return err
			}
			defer conn.Close()
			handleConnection(conn, stream)
		}
	} else {
		if c.URL.Scheme == "tcp" {
			listener, err := net.Listen("tcp", fmt.Sprintf("%s:%s", c.URL.Hostname(), c.URL.Port()))
			if err != nil {
				return err
			}
			defer listener.Close()
			for {
				conn, err := listener.Accept()
				defer conn.Close()
				if err != nil {
					return err
				}
				go handleConnection(conn, stream)
			}
		} else if c.URL.Scheme == "udp" {
			conn, err := net.ListenPacket("udp", fmt.Sprintf("%s:%s", c.URL.Hostname(), c.URL.Port()))
			if err != nil {
				return err
			}
			defer conn.Close()
			buffer := make([]byte, 65507)
			for {
				size, _, err := conn.ReadFrom(buffer)
				if err != nil {
					return err
				}
				stream <- buffer[:size]
			}
		}
	}
	return nil
}
