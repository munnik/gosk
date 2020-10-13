package keyvalue

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/jackc/pgx/v4"

	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk"
	"go.nanomsg.org/mangos/v3"
)

// Store saves all received messages in the database
func Store(socket mangos.Socket) {
	conn, err := pgx.Connect(context.Background(), "postgresql://gosk:gosk@localhost:5432")
	if err != nil {
		log.Fatal(err)
	}
	query := "insert into key_value_data (_time, _key, _context, _path, _value) values ($1, $2, $3, $4, $5)"
	for {
		raw, err := socket.Recv()
		if err != nil {
			log.Fatal(err)
		}
		m, err := nanomsg.Parse(raw)
		if err != nil {
			log.Fatal(err)
		}
		var signalKValueStruct signalk.Value
		json.Unmarshal(m.Payload, &signalKValueStruct)
		if len(signalKValueStruct.Path) == 0 || signalKValueStruct.Value == "" {
			continue
		}
		value, err := json.Marshal(signalKValueStruct.Value)
		if err != nil {
			log.Fatalf("Can't represent %v in json", signalKValueStruct.Value)
		}
		path := strings.Join(signalKValueStruct.Path, ".")
		if signalKValueStruct.Context == "" {
			// TODO this needs to be injected via config
			signalKValueStruct.Context = "vessels.urn:mrn:imo:mmsi:244770688"
		}
		if _, err = conn.Exec(context.Background(), query, m.Time, m.HeaderSegments, signalKValueStruct.Context, path, value); err != nil {
			log.Fatal(err)
		}
	}
}
