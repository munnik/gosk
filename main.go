package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/munnik/gosk/collector"
	nmeaCollector "github.com/munnik/gosk/collector/nmea0183"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk/mapper"

	_ "go.nanomsg.org/mangos/v3/transport/all"
)

func main() {
	f := setupLogging()
	defer f.Close()

	var tcpCollector collector.Collector
	tcpCollector = nmeaCollector.NewTCPCollector("192.168.1.151", 10110, "Wheelhouse")
	tcpCollectorPublisher := nanomsg.NewPub("tcp://127.0.0.1:40900")
	defer tcpCollectorPublisher.Close()
	go tcpCollector.Collect(nanomsg.Writer{Socket: tcpCollectorPublisher})

	collectorProxy := nanomsg.NewPubSubProxy("tcp://127.0.0.1:40899")
	defer collectorProxy.Close()
	collectorProxy.AddSubscriber("tcp://127.0.0.1:40900")

	mapperSubscriber := nanomsg.NewSub("tcp://127.0.0.1:40900", []byte("collector/"))
	defer mapperSubscriber.Close()
	mapMessage(nanomsg.Reader{Socket: mapperSubscriber}) // subscribe to the proxy
}

func mapMessage(reader io.Reader) {
	const bufferSize = 1024
	buffer := make([]byte, bufferSize)
	for {
		n, err := reader.Read(buffer)
		if err != nil {
			log.Fatal(err)
		}
		split := bytes.SplitN(buffer[0:n], []byte("\x00"), 2)
		if len(split) != 2 {
			log.Fatal("Could not find separator character in received message")
		}
		msgHeader := bytes.Split(split[0], []byte("/"))
		if len(msgHeader) < 3 {
			log.Fatal("Not enough information in the message header")
		}
		delta, err := mapper.DeltaFromData(split[1], string(msgHeader[1]), string(msgHeader[2]))
		if err != nil {
			log.Println(err)
		}
		json, err := json.Marshal(delta)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("Received a delta", string(json))
	}
}

func setupLogging() *os.File {
	var err error
	f, err := os.Create("logs/output.txt")
	if err != nil {
		log.Fatalf("Error creating log file: %v", err)
	}

	log.SetOutput(f)
	return f
}
