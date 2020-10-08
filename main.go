package main

import (
	"encoding/json"
	"io"
	"log"

	"github.com/munnik/gosk/collector/nmea"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk/mapper"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/pub"
	"go.nanomsg.org/mangos/v3/protocol/sub"
	_ "go.nanomsg.org/mangos/v3/transport/all"
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
	if err = subSocket.SetOption(mangos.OptionSubscribe, []byte("")); err != nil {
		log.Fatal("Could not subscribe to anything")
	}

	// fileCollector := nmea.FileCollector{
	// 	Config: nmea.FileConfig{
	// 		Path:        "data/output.nmea",
	// 		Interval:    time.Millisecond * 200,
	// 		LinesAtOnce: 3,
	// 	},
	// }
	// go fileCollector.Collect(nanomsg.Writer{Socket: pubSocket})

	tcpCollector := nmea.TCPCollector{
		Config: nmea.TCPConfig{
			Host: "192.168.1.151",
			Port: 10110,
		},
	}
	go tcpCollector.Collect(nanomsg.Writer{Socket: pubSocket})

	receiveMessages(nanomsg.Reader{Socket: subSocket})
}

func receiveMessages(reader io.Reader) {
	const bufferSize = 1024
	buffer := make([]byte, bufferSize)
	for {
		n, err := reader.Read(buffer)
		if err != nil {
			log.Fatal(err)
		}
		delta, err := mapper.DeltaFromData(buffer[0:n], mapper.NMEAType)
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
