package message

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Raw struct {
	Connector string    `json:"connector"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Uuid      uuid.UUID `json:"uuid"`
	Value     []byte    `json:"value"`
}

// ConnectorStatusType marks a Raw message as a connector reporting its own
// online/offline status (see connector/main.go's process), rather than
// protocol data for a mapper to decode. Its Value is one of the
// ConnectorStatus* constants below. A mapper that wants to surface this as
// a SignalK notification checks for this Type before treating a Raw
// message as its usual protocol payload - see mapper/main.go's process.
const ConnectorStatusType = "connectorStatus"

// Values Raw.Value holds when Type is ConnectorStatusType.
// ConnectorStatusDisconnectedOrNoData covers both a connector that never
// managed to connect at all and one that connected but has since stopped
// receiving data - see connector/main.go's process, which repeats this
// report every config.Timeout for as long as either is true, rather than
// reporting it once.
const (
	ConnectorStatusConnectedAndData     = "connectedAndData"
	ConnectorStatusDisconnectedOrNoData = "disconnectedOrNoData"
)

func NewRaw() *Raw {
	return &Raw{
		Uuid:      uuid.New(),
		Timestamp: time.Now(),
	}
}

func (r *Raw) WithConnector(c string) *Raw {
	r.Connector = c
	return r
}

func (r *Raw) WithType(t string) *Raw {
	r.Type = t
	return r
}

func (r *Raw) WithValue(v []byte) *Raw {
	r.Value = v
	return r
}

// rawWire is Raw's JSON shape, field-for-field and in the same field order
// the old map[string]string-based MarshalJSON produced (Go sorts map keys
// alphabetically when marshaling, and connector/timestamp/type/uuid/value
// already sort that way) - so this produces byte-identical output, just
// via the struct encoder instead of allocating and sorting a map on every
// single call. Value stays []byte rather than a pre-encoded string:
// encoding/json already base64-encodes a []byte field itself (with the
// same standard, padded alphabet base64.StdEncoding.EncodeToString would
// produce), writing straight into the output buffer instead of needing an
// intermediate string allocation. On node-hbr-rpa10, MarshalJSON on Raw
// values - one of the hottest paths in the whole pipeline, called on
// every 2kHz sample from the shaft power meter - accounted for close to
// half of a connector process's total CPU time under load (see gosk's
// performance investigation), and virtually all of that was this
// map/sort overhead, not the actual work of producing the bytes.
//
// A nil []byte and a non-nil, empty []byte marshal differently (null vs
// "") - the old base64.StdEncoding.EncodeToString(nil)-into-a-string-map
// approach always produced "" - so MarshalJSON normalizes a nil r.Value
// to an empty (non-nil) slice before assigning it here, to keep matching
// that regardless of which one this particular Raw happens to hold.
//
//easyjson:json
type rawWire struct {
	Connector string `json:"connector"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Uuid      string `json:"uuid"`
	Value     []byte `json:"value"`
}

func (r Raw) MarshalJSON() ([]byte, error) {
	value := r.Value
	if value == nil {
		value = []byte{}
	}
	// rawWire.MarshalJSON is easyjson-generated - calling it directly,
	// rather than json.Marshal(rawWire{...}), skips encoding/json's own
	// per-call interface-detection/reflection setup on top of it.
	return rawWire{
		Connector: r.Connector,
		Timestamp: r.Timestamp.UTC().Format(time.RFC3339Nano),
		Type:      r.Type,
		Uuid:      r.Uuid.String(),
		Value:     value,
	}.MarshalJSON()
}

func (r *Raw) UnmarshalJSON(data []byte) error {
	var err error
	var j map[string]string
	if err = json.Unmarshal(data, &j); err != nil {
		return err
	}
	for _, key := range []string{"connector", "timestamp", "type", "uuid", "value"} {
		if _, ok := j[key]; !ok {
			return fmt.Errorf("the key '%v' is missing in the json message %+v", key, j)
		}
	}
	r.Connector = j["connector"]
	if r.Timestamp, err = time.Parse(time.RFC3339Nano, j["timestamp"]); err != nil {
		return err
	}
	r.Type = j["type"]
	if r.Uuid, err = uuid.Parse(j["uuid"]); err != nil {
		return err
	}
	if r.Value, err = base64.StdEncoding.DecodeString(j["value"]); err != nil {
		return err
	}

	return nil
}

func (r Raw) Equals(other Raw) bool {
	return r.Connector == other.Connector &&
		r.Type == other.Type &&
		bytes.Equal(r.Value, other.Value)
}
