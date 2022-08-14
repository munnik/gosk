package message

import (
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

func (r Raw) MarshalJSON() ([]byte, error) {
	var result map[string]string = make(map[string]string)
	result["connector"] = r.Connector
	result["timestamp"] = r.Timestamp.UTC().Format(time.RFC3339Nano)
	result["type"] = r.Type
	result["uuid"] = r.Uuid.String()
	result["value"] = base64.StdEncoding.EncodeToString(r.Value)
	return json.Marshal(&result)
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
