package keyvalue

import (
	"context"
	"encoding/json"
	"log"

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
	queryWithoutContext := "insert into key_value_data (_time, _key, _path, _value) values ($1, $2, $3, $4)"
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
		if signalKValueStruct.Path == "" || signalKValueStruct.Value == "" {
			continue
		}
		value, err := json.Marshal(signalKValueStruct.Value)
		if err != nil {
			log.Fatalf("Can't represent %v in json", signalKValueStruct.Value)
		}
		if signalKValueStruct.Context == "" {
			if _, err = conn.Exec(context.Background(), queryWithoutContext, m.Time, m.HeaderSegments, signalKValueStruct.Path, value); err != nil {
				log.Fatal(err)
			}
		} else {
			if _, err = conn.Exec(context.Background(), query, m.Time, m.HeaderSegments, signalKValueStruct.Context, signalKValueStruct.Path, value); err != nil {
				log.Fatal(err)
			}
		}
	}
}
