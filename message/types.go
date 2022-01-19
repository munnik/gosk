package message

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

type RawExtraInfo interface{}

type Raw struct {
	Timestamp time.Time
	Collector string
	ExtraInfo RawExtraInfo `json:"extra_info",omitempty`
	Value     []byte
}

type Mapped struct {
	Context string
	Updates []struct {
		Source    string
		Timestamp time.Time
		Values    []struct {
			Path  string
			Value string
		}
	}
}

func (m *Raw) MarshalJSON() ([]byte, error) {
	var result map[string]string = make(map[string]string)
	result["timestamp"] = m.Timestamp.UTC().Format(time.RFC3339Nano)
	result["collector"] = m.Collector
	result["value"] = base64.StdEncoding.EncodeToString(m.Value)
	return json.Marshal(&result)
}

func (m *Raw) UnmarshalJSON(data []byte) (err error) {
	return
}
