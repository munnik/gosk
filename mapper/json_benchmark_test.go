package mapper_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
)

// payload is the three keys json_test.yaml maps, padded with extra keys the
// mappings ignore. "small" is the shape of the torque message the suite
// already covers; "wide" is the shape of a real sensor message, which is
// mostly fields nobody maps - an IO-Link vibration sensor on
// node-hbr-rpa10 publishes 31 items in ~1.7kB and three of them are read.
func payload(extraKeys int) []byte {
	var b strings.Builder
	b.WriteString(`{"pwr":"8409.6","spd":"980","trq":"82.29"`)
	for i := range extraKeys {
		fmt.Fprintf(&b, `,"Vibration Status Bits Reserved Channel %02d":false`, i)
	}
	b.WriteString("}")
	return []byte(b.String())
}

// BenchmarkJSONDoMap maps one message through the three mappings in
// json_test.yaml. DoMap used to json.Unmarshal the payload once per
// mapping, so this measured three decodes of the same bytes - the cost
// that matters here is per byte of payload, not per mapped value, which is
// why the wide case is the interesting one.
func BenchmarkJSONDoMap(b *testing.B) {
	for _, c := range []struct {
		name      string
		extraKeys int
	}{
		{"small", 0},
		{"wide", 28},
	} {
		b.Run(c.name, func(b *testing.B) {
			m, err := mapper.NewJSONMapper(
				config.MapperConfig{Context: "testingContext"},
				config.NewJSONMappingConfig("json_test.yaml"),
			)
			if err != nil {
				b.Fatal(err)
			}
			raw := message.NewRaw().
				WithConnector("testingConnector").
				WithType(config.JSONType).
				WithValue(payload(c.extraKeys))
			raw.Uuid = uuid.Nil
			raw.Timestamp = time.Now()

			b.SetBytes(int64(len(raw.Value)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := m.DoMap(raw); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
