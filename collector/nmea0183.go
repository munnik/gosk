package collector

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/tarm/serial"

	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk/mapper"
	"go.nanomsg.org/mangos/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
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

// NewNMEA0183NetworkCollector creates an instance of a TCP collector
func NewNMEA0183NetworkCollector(url *url.URL, dial bool, name string) *NMEA0183NetworkCollector {
	return &NMEA0183NetworkCollector{
		URL:  url,
		Dial: dial,
		Name: name,
	}
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
func (c *NMEA0183NetworkCollector) Collect(socket mangos.Socket) {
	stream := make(chan []byte, 1)

	go receiveFromNetwork(c.URL, c.Dial, stream)
	processStream(stream, socket, c.Name)
}

// Collect start the collection process and keeps running as long as there is data available
func (c *NMEA0183FileCollector) Collect(socket mangos.Socket) {
	stream := make(chan []byte, 1)

	go receiveFromFile(c.Path, c.BaudRate, stream)
	processStream(stream, socket, c.Name)
}

func processStream(stream <-chan []byte, socket mangos.Socket, name string) {
	for payload := range stream {
		log.Debug(fmt.Sprintf("Retrieving a message from the stream: %s", payload))
		m := &nanomsg.RawData{
			Header: &nanomsg.Header{
				HeaderSegments: []string{"collector", mapper.NMEA0183Type, name},
			},
			Timestamp: timestamppb.Now(),
			Payload:   payload,
		}
		toSend, err := proto.Marshal(m)
		if err != nil {
			log.Warn(err)
			continue
		}
		if err := socket.Send(toSend); err != nil {
			log.Warn(err)
			continue
		}
		log.Debug(fmt.Sprintf("Forwarded the message on the Nanomsg socket: %s", payload))
	}
}

func receiveFromFile(path string, baudRate int, stream chan<- []byte) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeCharDevice == os.ModeCharDevice {
		s, err := serial.OpenPort(&serial.Config{Name: path, Baud: baudRate})
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
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			stream <- scanner.Bytes()
		}
	}

	close(stream)
	return nil
}

func receiveFromNetwork(url *url.URL, dial bool, stream chan<- []byte) error {
	log.Info(fmt.Sprintf("Start to collect data from the network using the host and port %s:%s", url.Hostname(), url.Port()))
	if dial {
		if url.Scheme == "udp" || url.Scheme == "tcp" {
			conn, err := net.Dial(url.Scheme, fmt.Sprintf("%s:%s", url.Hostname(), url.Port()))
			if err != nil {
				return err
			}
			defer conn.Close()
			handleConnection(conn, stream)
		}
	} else {
		if url.Scheme == "tcp" {
			listener, err := net.Listen("tcp", fmt.Sprintf("%s:%s", url.Hostname(), url.Port()))
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
		} else if url.Scheme == "udp" {
			conn, err := net.ListenPacket("udp", fmt.Sprintf("%s:%s", url.Hostname(), url.Port()))
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
	close(stream)
	return nil
}

func handleConnection(conn net.Conn, payloadStream chan<- []byte) {
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		buffer := scanner.Bytes()
		payloadStream <- buffer
	}
}
