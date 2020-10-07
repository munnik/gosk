package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-zeromq/zmq4"
	"github.com/munnik/gosk/collector/nmea"
)

func main() {
	ctx := context.Background()

	publisherSocket := zmq4.NewPub(ctx)
	defer publisherSocket.Close()
	if err := publisherSocket.Listen("tcp://127.0.0.1:3000"); err != nil {
		log.Fatal(err)
	}

	subscriberSocket := zmq4.NewSub(ctx)
	subscriberSocket.SetOption(zmq4.OptionSubscribe, "NMEA")
	defer subscriberSocket.Close()
	if err := subscriberSocket.Dial("tcp://127.0.0.1:3000"); err != nil {
		log.Fatal(err)
	}

	if !publisherSocket.Type().IsCompatible(subscriberSocket.Type()) {
		log.Fatalf("%T is not compatible with %T\n", publisherSocket, subscriberSocket)
	}
	fmt.Printf("%T is compatible with %T\n", publisherSocket, subscriberSocket)

	fc := nmea.FileCollector{
		Config: nmea.FileConfig{
			Path:        "data/output.nmea",
			Interval:    time.Millisecond * 200,
			LinesAtOnce: 3,
		},
	}

	go fc.Collect(publisherSocket)

	for {
		message, err := subscriberSocket.Recv()
		if err != nil {
			log.Print(err)
		}
		log.Println(string(message.Bytes()))
	}
}
