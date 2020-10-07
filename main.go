package main

import (
	"log"
	"time"

	"github.com/munnik/gosk/collector/nmea"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/pub"
	"go.nanomsg.org/mangos/v3/protocol/sub"
)

func main() {
	var pubSocket mangos.Socket
	var err error
	pubSocket, err = pub.NewSocket()
	if err != nil {
		log.Fatalf("Can't create pubSocket: %s", err)
	}
	defer pubSocket.Close()
	if err := pubSocket.Listen("tcp://127.0.0.1:40899"); err != nil {
		log.Fatal(err)
	}

	var subSocket mangos.Socket
	subSocket, err = sub.NewSocket()
	if err != nil {
		log.Fatalf("Can't create subSocket: %s", err)
	}
	defer subSocket.Close()
	if err := subSocket.Dial("tcp://127.0.0.1:40899"); err != nil {
		log.Fatal(err)
	}

	fc := nmea.FileCollector{
		Config: nmea.FileConfig{
			Path:        "data/output.nmea",
			Interval:    time.Millisecond * 200,
			LinesAtOnce: 3,
		},
	}

	go fc.Collect(pubSocket)

	for {
		message, err := subSocket.Recv()
		if err != nil {
			log.Print(err)
		}
		log.Println(string(message))
	}
}
