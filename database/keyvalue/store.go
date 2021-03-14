package keyvalue

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/jackc/pgx/v4"
	"google.golang.org/protobuf/proto"

	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
)

// Store saves all received messages in the database
func Store(socket mangos.Socket) {
	conn, err := pgx.Connect(context.Background(), "postgresql://gosk:gosk@localhost:5432")
	if err != nil {
		log.Fatal(err)
	}
	query := "insert into key_value_data (_time, _key, _context, _path, _value) values ($1, $2, $3, $4, $5)"
	m := &nanomsg.MappedData{}
	for {
		received, err := socket.Recv()
		if err != nil {
			log.Fatal(err)
		}
		if err := proto.Unmarshal(received, m); err != nil {
			log.Fatal(err)
		}
		if m.Datatype == nanomsg.DOUBLE {
			if _, err = conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, m.Path, fmt.Sprintf("%f", m.DoubleValue)); err != nil {
				log.Fatal(err)
			}
		} else if m.Datatype == nanomsg.STRING {
			if _, err = conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, m.Path, m.StringValue); err != nil {
				log.Fatal(err)
			}
		} else if m.Datatype == nanomsg.POSITION {
			if _, err = conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, m.Path, m.PositionValue.String()); err != nil {
				log.Fatal(err)
			}
		} else if m.Datatype == nanomsg.LENGTH {
			if _, err = conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, m.Path, m.LengthValue.String()); err != nil {
				log.Fatal(err)
			}
		}
	}
}
