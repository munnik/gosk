package raw

import (
	"context"
	"log"

	"github.com/jackc/pgx/v4"
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
)

// Store saves all received messages in the database
func Store(socket mangos.Socket) {
	conn, err := pgx.Connect(context.Background(), "postgresql://gosk:gosk@localhost:5432")
	if err != nil {
		log.Fatal(err)
	}
	query := "insert into raw_data (_time, _key, _value) values ($1, $2, $3)"
	for {
		raw, err := socket.Recv()
		if err != nil {
			log.Fatal(err)
		}
		m, err := nanomsg.Parse(raw)
		if err != nil {
			log.Fatal(err)
		}
		if _, err = conn.Exec(context.Background(), query, m.Time, m.HeaderSegments, m.Payload); err != nil {
			log.Fatal(err)
		}
	}
}
