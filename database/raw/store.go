package raw

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/golang/protobuf/proto"
	"github.com/jackc/pgx/v4"
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
)

// Store saves all received messages in the database
func Store(socket mangos.Socket) {
	conn, err := pgx.Connect(context.Background(), "postgresql://gosk:gosk@localhost:5432")
	if err != nil {
		log.Fatal(err)
		panic(err)
	}
	query := "insert into raw_data (_time, _key, _value) values ($1, $2, $3)"
	m := &nanomsg.RawData{}
	for {
		received, err := socket.Recv()
		if err != nil {
			log.Warn(err)
		}
		if err := proto.Unmarshal(received, m); err != nil {
			log.Warn(err)
		}
		if _, err = conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Payload); err != nil {
			log.Warn(err)
		}
	}
}
