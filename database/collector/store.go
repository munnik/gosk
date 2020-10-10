package collector

import (
	"context"
	"log"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
)

var conn *pgxpool.Pool

func init() {
	var err error
	conn, err = pgxpool.Connect(context.Background(), "postgresql://gosk:gosk@localhost:5432")
	if err != nil {
		log.Fatal(err)
	}
}

// Store saves all received messages in the database
func Store(socket mangos.Socket) {
	for {
		raw, err := socket.Recv()
		if err != nil {
			log.Fatal(err)
		}
		m, err := nanomsg.Parse(raw)
		if err != nil {
			log.Fatal(err)
		}
		if _, err = conn.Exec(context.Background(), `insert into raw_data (_time, _key, _value) values ($1, $2, $3)`, m.Time, m.HeaderSegments, m.Payload); err != nil {
			log.Fatal(err)
		}
	}
}
