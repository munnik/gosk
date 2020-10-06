package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/munnik/gosk/signalk/parser"
)

func fileReader(file string, interval time.Duration, linesAtOnce int, c chan<- []byte) {
	defer close(c)
	data, err := ioutil.ReadFile(file)

	if err != nil {
		log.Fatal(err)
		return
	}

	lines := strings.Split(string(data[:]), "\n")
	var lineCounter int
	for _, line := range lines {
		c <- []byte(line)
		lineCounter++
		if lineCounter%linesAtOnce == 0 {
			time.Sleep(interval)
		}
	}
}

func parse(c chan []byte, dataType string) {
	for sentence := range c {
		delta, err := parser.DeltaFromData(sentence, dataType)
		if err != nil {
			log.Println(err)
			continue
		}

		json, err := json.Marshal(delta)
		if err != nil {
			log.Fatal(err)
			continue
		}
		fmt.Println("Got a new delta", string(json))
	}
}

func main() {
	c := make(chan []byte)
	go fileReader("data/output.nmea", time.Millisecond*100, 3, c)
	parse(c, parser.NMEAType)
}
