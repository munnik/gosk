package nanomsg

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Used identify header segments
const (
	HEADERSEGMENTPROCESS  = 0
	HEADERSEGMENTPROTOCOL = 1
	HEADERSEGMENTSOURCE   = 2
)

// Used for splitting the message
const (
	MESSAGESEPERATOR = "\x00"
	HEADERSEPERATOR  = "/"
)

// Message gives easy access to the Header, HeaderSegments and the Payload. Use Parse to parse a raw message
type Message struct {
	Header         []byte
	HeaderSegments [][]byte
	Time           time.Time
	Payload        []byte
}

// Parse a raw message
func Parse(raw []byte) (Message, error) {
	msgSplit := bytes.SplitN(raw, []byte(MESSAGESEPERATOR), 3)
	if len(msgSplit) < 3 {
		return Message{}, errors.New("Invalid message syntax, message should at least contain two null characters")
	}
	unixNano, err := strconv.ParseInt(string(msgSplit[1]), 10, 64)
	if err != nil {
		return Message{}, err
	}
	return Message{
		Header:         msgSplit[0],
		HeaderSegments: bytes.Split(msgSplit[0], []byte(HEADERSEPERATOR)),
		Time:           time.Unix(0, unixNano),
		Payload:        msgSplit[2],
	}, nil
}

// Create builds a new message
func Create(payload []byte, time time.Time, headerSegments ...[]byte) Message {
	headerSegmentsAsString := make([]string, len(headerSegments))
	for index, headerSegment := range headerSegments {
		headerSegmentsAsString[index] = string(headerSegment)
	}
	header := []byte(strings.Join(headerSegmentsAsString, HEADERSEPERATOR))
	return Message{
		Payload:        payload,
		Time:           time,
		HeaderSegments: headerSegments,
		Header:         header,
	}
}

func (m Message) String() string {
	return string(m.Header) + MESSAGESEPERATOR + strconv.FormatInt(m.Time.UTC().UnixNano(), 10) + MESSAGESEPERATOR + string(m.Payload)
}
