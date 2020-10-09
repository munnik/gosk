package main

import (
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/munnik/gosk/collector/nmea"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk/mapper"

	_ "go.nanomsg.org/mangos/v3/transport/all"
)

func main() {
	setupLogging()

	tcpCollector := nmea.TCPCollector{
		Config: nmea.TCPConfig{
			Host: "192.168.1.151",
			Port: 10110,
		},
	}
	tcpCollectorPublisher := nanomsg.NewPub("tcp://127.0.0.1:40900")
	defer tcpCollectorPublisher.Close()
	go tcpCollector.Collect(nanomsg.Writer{Socket: tcpCollectorPublisher})

	collectorProxy := nanomsg.NewPubSubProxy("tcp://127.0.0.1:40899")
	defer collectorProxy.Close()
	collectorProxy.AddSubscriber("tcp://127.0.0.1:40900", []byte(""))

	collectorSubscriber := nanomsg.NewSub("tcp://127.0.0.1:40900", []byte(""))
	defer collectorSubscriber.Close()
	receiveMessages(nanomsg.Reader{Socket: collectorSubscriber}) // subscribe to the proxy
}

func receiveMessages(reader io.Reader) {
	const bufferSize = 1024
	buffer := make([]byte, bufferSize)
	for {
		n, err := reader.Read(buffer)
		if err != nil {
			log.Fatal(err)
		}
		delta, err := mapper.DeltaFromData(buffer[0:n], mapper.NMEAType, "NMEA0183 Collector")
		if err != nil {
			log.Print(err)
		}
		json, err := json.Marshal(delta)
		if err != nil {
			log.Print(err)
		}
		log.Println("Received a delta", string(json))
	}
}

func setupLogging() {
	var err error
	f, err := os.Create("logs/output.txt")
	if err != nil {
		log.Fatalf("Error creating log file: %v", err)
	}
	defer f.Close()

	log.SetOutput(f)
}
