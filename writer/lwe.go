package writer

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

var talkerMulticastMap = map[string]*net.UDPAddr{
	"AG": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"AI": {IP: net.ParseIP("239.192.0.2"), Port: 60002},
	"AP": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"BI": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"BN": {IP: net.ParseIP("239.192.0.5"), Port: 60005},
	"CD": {IP: net.ParseIP("239.192.0.6"), Port: 60006},
	"CR": {IP: net.ParseIP("239.192.0.6"), Port: 60006},
	"CS": {IP: net.ParseIP("239.192.0.6"), Port: 60006},
	"CT": {IP: net.ParseIP("239.192.0.6"), Port: 60006},
	"CV": {IP: net.ParseIP("239.192.0.6"), Port: 60006},
	"CX": {IP: net.ParseIP("239.192.0.6"), Port: 60006},
	"DF": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"DU": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"EC": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"EI": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"EP": {IP: net.ParseIP("239.192.0.6"), Port: 60006},
	"ER": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"FD": {IP: net.ParseIP("239.192.0.5"), Port: 60005},
	"FE": {IP: net.ParseIP("239.192.0.5"), Port: 60005},
	"FR": {IP: net.ParseIP("239.192.0.5"), Port: 60005},
	"FS": {IP: net.ParseIP("239.192.0.5"), Port: 60005},
	"GA": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"GL": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"GN": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"GP": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"HC": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"HD": {IP: net.ParseIP("239.192.0.5"), Port: 60005},
	"HE": {IP: net.ParseIP("239.192.0.3"), Port: 60003},
	"HF": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"HN": {IP: net.ParseIP("239.192.0.3"), Port: 60003},
	"HS": {IP: net.ParseIP("239.192.0.5"), Port: 60005},
	"II": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"IN": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"LC": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"NL": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"RA": {IP: net.ParseIP("239.192.0.2"), Port: 60002},
	"RC": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"SD": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"SG": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"SI": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"SS": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"TI": {IP: net.ParseIP("239.192.0.3"), Port: 60003},
	"U0": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"U1": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"U2": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"U3": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"U4": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"U5": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"U6": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"U7": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"U8": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"U9": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"UP": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"VD": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"VM": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"VR": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"VW": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"WD": {IP: net.ParseIP("239.192.0.5"), Port: 60005},
	"WI": {IP: net.ParseIP("239.192.0.4"), Port: 60004},
	"WL": {IP: net.ParseIP("239.192.0.5"), Port: 60005},
	"YX": {IP: net.ParseIP("239.192.0.1"), Port: 60001},
	"ZA": {IP: net.ParseIP("239.192.0.7"), Port: 60007},
	"ZC": {IP: net.ParseIP("239.192.0.7"), Port: 60007},
	"ZQ": {IP: net.ParseIP("239.192.0.7"), Port: 60007},
	"ZV": {IP: net.ParseIP("239.192.0.7"), Port: 60007},
}

type LWEWriter struct {
	DestinationIdentification string
	SourceIdentification      string
	IncludeTimestamp          bool
	IncludeLineCount          bool
	lineCount                 uint32
	mu                        sync.Mutex
}

func NewLWEWriter(c *config.LWEConfig) *LWEWriter {
	return &LWEWriter{
		DestinationIdentification: c.DestinationIdentification,
		SourceIdentification:      c.SourceIdentification,
		IncludeTimestamp:          c.IncludeTimestamp,
		IncludeLineCount:          c.IncludeLineCount,
	}
}
func (w *LWEWriter) WriteRaw(subscriber mangos.Socket) {
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		raw := message.Raw{}
		if err := json.Unmarshal(received, &raw); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		go w.multicast(raw)
	}
}

func (w *LWEWriter) multicast(raw message.Raw) {
	talkerID := string(raw.Value)[:2]
	if _, ok := talkerMulticastMap[talkerID]; !ok {
		return
	}

	conn, err := net.DialUDP("udp4", nil, talkerMulticastMap[talkerID])
	if err != nil {
		return
	}

	conn.Write(append(append(append([]byte("UdPbC\x00"), w.createTagBlock(raw)...), raw.Value...), []byte("\r\n")...))
}

func (w *LWEWriter) createTagBlock(raw message.Raw) []byte {
	tagBlock := ""
	if w.DestinationIdentification != "" {
		tagBlock += "d:" + w.DestinationIdentification + ","
	}
	if w.SourceIdentification != "" {
		tagBlock += "s:" + w.SourceIdentification + ","
	}
	if w.IncludeTimestamp {
		tagBlock += "t:" + fmt.Sprint(raw.Timestamp.Unix()) + ","
	}
	if w.IncludeLineCount {
		w.mu.Lock()
		if w.lineCount >= 1000 {
			w.lineCount = 0
		}
		tagBlock += "l:" + fmt.Sprint(w.lineCount) + ","
		w.lineCount += 1
		w.mu.Unlock()
	}
	if tagBlock == "" {
		return []byte{}
	}

	tagBlock = tagBlock[:len(tagBlock)-1] // remove last comma

	tagBlockChecksum := 0
	for _, c := range tagBlock {
		tagBlockChecksum ^= int(c)
	}
	return []byte("\\" + tagBlock + "*" + fmt.Sprint(tagBlockChecksum) + "\\")
}
