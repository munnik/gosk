package main

import (
	"time"

	"github.com/munnik/gosk/collector"
	nmeaCollector "github.com/munnik/gosk/collector/nmea0183"
	keyvalueStoreCollector "github.com/munnik/gosk/database/keyvalue"
	rawStoreCollector "github.com/munnik/gosk/database/raw"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk/mapper"

	_ "go.nanomsg.org/mangos/v3/transport/all"
)

func main() {
	var tcpCollector collector.Collector
	tcpCollector = nmeaCollector.NewTCPCollector("192.168.1.151", 10110, "Wheelhouse")
	tcpCollectorPublisher := nanomsg.NewPub("tcp://127.0.0.1:6000")
	defer tcpCollectorPublisher.Close()
	go tcpCollector.Collect(tcpCollectorPublisher)

	// TODO create a Modbus TCP collector

	collectorProxy := nanomsg.NewPubSubProxy("tcp://127.0.0.1:6100")
	defer collectorProxy.Close()
	collectorProxy.AddSubscriber("tcp://127.0.0.1:6000")

	rawStoreSubscriber := nanomsg.NewSub("tcp://127.0.0.1:6100", []byte(""))
	defer rawStoreSubscriber.Close()
	go rawStoreCollector.Store(rawStoreSubscriber) // subscribe to the proxy

	mapperSubscriber := nanomsg.NewSub("tcp://127.0.0.1:6100", []byte(""))
	defer mapperSubscriber.Close()
	mapperPublisher := nanomsg.NewPub("tcp://127.0.0.1:6200")
	defer mapperPublisher.Close()
	go mapper.Map(mapperSubscriber, mapperPublisher) // subscribe to the proxy

	keyvalueStoreSubscriber := nanomsg.NewSub("tcp://127.0.0.1:6200", []byte(""))
	defer keyvalueStoreSubscriber.Close()
	go keyvalueStoreCollector.Store(keyvalueStoreSubscriber)

	for {
		time.Sleep(time.Second)
	}
}
