package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/munnik/gosk/collector"
	nmeaCollector "github.com/munnik/gosk/collector/nmea0183"
	storeCollector "github.com/munnik/gosk/database/collector"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk/mapper"

	"go.nanomsg.org/mangos/v3"
	_ "go.nanomsg.org/mangos/v3/transport/all"
)

func main() {
	f := setupLogging()
	defer f.Close()

	var tcpCollector collector.Collector
	tcpCollector = nmeaCollector.NewTCPCollector("192.168.1.151", 10110, "Wheelhouse")
	tcpCollectorPublisher := nanomsg.NewPub("tcp://127.0.0.1:40899")
	defer tcpCollectorPublisher.Close()
	go tcpCollector.Collect(tcpCollectorPublisher)

	// TODO create a Modbus TCP collector

	collectorProxy := nanomsg.NewPubSubProxy("tcp://127.0.0.1:40900")
	defer collectorProxy.Close()
	collectorProxy.AddSubscriber("tcp://127.0.0.1:40899")

	storeSubscriber := nanomsg.NewSub("tcp://127.0.0.1:40900", []byte("collector/"))
	defer storeSubscriber.Close()
	go storeCollector.Store(storeSubscriber) // subscribe to the proxy

	mapperSubscriber := nanomsg.NewSub("tcp://127.0.0.1:40900", []byte("collector/"))
	defer mapperSubscriber.Close()
	mapMessage(mapperSubscriber) // subscribe to the proxy
}

func mapMessage(socket mangos.Socket) {
	for {
		raw, err := socket.Recv()
		if err != nil {
			log.Fatal(err)
		}
		m, err := nanomsg.Parse(raw)
		if err != nil {
			log.Fatal(err)
		}
		delta, err := mapper.DeltaFromMessage(m)
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
