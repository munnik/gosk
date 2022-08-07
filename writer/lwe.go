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

const (
	MISCAddress = "239.192.0.1"
	MISCPort    = 60001
	TGTDAddress = "239.192.0.2"
	TGTDPort    = 60002
	SATDAddress = "239.192.0.3"
	SATDPort    = 60003
	NAVDAddress = "239.192.0.4"
	NAVDPort    = 60004
	VDRDAddress = "239.192.0.5"
	VDRDPort    = 60005
	RCOMAddress = "239.192.0.6"
	RCOMPort    = 60006
	TIMEAddress = "239.192.0.7"
	TIMEPort    = 60007
	PROPAddress = "239.192.0.8"
	PROPPort    = 60008
	USR1Address = "239.192.0.9"
	USR1Port    = 60009
	USR2Address = "239.192.0.10"
	USR2Port    = 60010
	USR3Address = "239.192.0.11"
	USR3Port    = 60011
	USR4Address = "239.192.0.12"
	USR4Port    = 60012
	USR5Address = "239.192.0.13"
	USR5Port    = 60013
	USR6Address = "239.192.0.14"
	USR6Port    = 60014
	USR7Address = "239.192.0.15"
	USR7Port    = 60015
	USR8Address = "239.192.0.16"
	USR8Port    = 60016
)

var talkerMulticastMap = map[string]*net.UDPAddr{
	"AG": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"AI": {IP: net.ParseIP(TGTDAddress), Port: TGTDPort},
	"AP": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"BI": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"BN": {IP: net.ParseIP(VDRDAddress), Port: VDRDPort},
	"CD": {IP: net.ParseIP(RCOMAddress), Port: RCOMPort},
	"CR": {IP: net.ParseIP(RCOMAddress), Port: RCOMPort},
	"CS": {IP: net.ParseIP(RCOMAddress), Port: RCOMPort},
	"CT": {IP: net.ParseIP(RCOMAddress), Port: RCOMPort},
	"CV": {IP: net.ParseIP(RCOMAddress), Port: RCOMPort},
	"CX": {IP: net.ParseIP(RCOMAddress), Port: RCOMPort},
	"DF": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"DU": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"EC": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"EI": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"EP": {IP: net.ParseIP(RCOMAddress), Port: RCOMPort},
	"ER": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"FD": {IP: net.ParseIP(VDRDAddress), Port: VDRDPort},
	"FE": {IP: net.ParseIP(VDRDAddress), Port: VDRDPort},
	"FR": {IP: net.ParseIP(VDRDAddress), Port: VDRDPort},
	"FS": {IP: net.ParseIP(VDRDAddress), Port: VDRDPort},
	"GA": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"GL": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"GN": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"GP": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"HC": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"HD": {IP: net.ParseIP(VDRDAddress), Port: VDRDPort},
	"HE": {IP: net.ParseIP(SATDAddress), Port: SATDPort},
	"HF": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"HN": {IP: net.ParseIP(SATDAddress), Port: SATDPort},
	"HS": {IP: net.ParseIP(VDRDAddress), Port: VDRDPort},
	"II": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"IN": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"LC": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"NL": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"RA": {IP: net.ParseIP(TGTDAddress), Port: TGTDPort},
	"RC": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"SD": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"SG": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"SI": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"SS": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"TI": {IP: net.ParseIP(SATDAddress), Port: SATDPort},
	"U0": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"U1": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"U2": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"U3": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"U4": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"U5": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"U6": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"U7": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"U8": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"U9": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"UP": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"VD": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"VM": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"VR": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"VW": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"WD": {IP: net.ParseIP(VDRDAddress), Port: VDRDPort},
	"WI": {IP: net.ParseIP(NAVDAddress), Port: NAVDPort},
	"WL": {IP: net.ParseIP(VDRDAddress), Port: VDRDPort},
	"YX": {IP: net.ParseIP(MISCAddress), Port: MISCPort},
	"ZA": {IP: net.ParseIP(TIMEAddress), Port: TIMEPort},
	"ZC": {IP: net.ParseIP(TIMEAddress), Port: TIMEPort},
	"ZQ": {IP: net.ParseIP(TIMEAddress), Port: TIMEPort},
	"ZV": {IP: net.ParseIP(TIMEAddress), Port: TIMEPort},
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
