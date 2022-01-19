package collector

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"net"
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

// Collector interface
type Collector interface {
	Collect(mangos.Socket)
}

func processStream(stream <-chan []byte, collector string, socket mangos.Socket) {
	var m message.Raw
	for payload := range stream {
		logger.GetLogger().Debug(
			"Received a message from the stream",
			zap.ByteString("Message", payload),
		)

		m = message.Raw{
			Timestamp: time.Now(),
			Collector: collector,
			Value:     base64.StdEncoding.EncodeToString(payload),
		}
		toSend, err := json.Marshal(m)
		if err != nil {
			logger.GetLogger().Warn(
				"Unable to marshall the message to ProtoBuffer",
				zap.ByteString("Message", payload),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := socket.Send(toSend); err != nil {
			logger.GetLogger().Warn(
				"Unable to send the message using NanoMSG",
				zap.ByteString("Message", payload),
				zap.String("Error", err.Error()),
			)
			continue
		}
		logger.GetLogger().Debug(
			"Send the message on the NanoMSG socket",
			zap.ByteString("Message", payload),
		)
	}
}

func handleConnection(conn net.Conn, payloadStream chan<- []byte) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		buffer := scanner.Bytes()
		payloadStream <- buffer
	}
	logger.GetLogger().Warn(
		"Could not read from connection",
		zap.Any("Connection", conn),
	)
}

func uint16ArrayToByteArray(in []uint16) []byte {
	result := make([]byte, 2*len(in))

	for i, v := range in {
		result[2*i] = byte((v & 0xff00) >> 8)
		result[2*i+1] = byte(v & 0xff)
	}
	return result
}
