package collector

import (
	"bufio"
	"net"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Collector interface
type Collector interface {
	Collect(mangos.Socket)
}

func processStream(stream <-chan []byte, messageType string, socket mangos.Socket, name string) {
	for payload := range stream {
		logger.GetLogger().Debug(
			"Received a message from the stream",
			zap.ByteString("Message", payload),
		)
		m := &nanomsg.RawData{
			Header: &nanomsg.Header{
				HeaderSegments: []string{"collector", messageType, name},
			},
			Timestamp: timestamppb.Now(),
			Payload:   payload,
		}
		toSend, err := proto.Marshal(m)
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
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		buffer := scanner.Bytes()
		payloadStream <- buffer
	}
}

func uint16sToBytes(in []uint16) []byte {
	result := make([]byte, 2*len(in))

	for i, v := range in {
		result[2*i] = byte((v & 0xff00) >> 8)
		result[2*i+1] = byte(v & 0xff)
	}
	return result
}
