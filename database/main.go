package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
)

// StoreRaw saves all received raw messages in the database
func StoreRaw(socket mangos.Socket) {
	conn, err := pgx.Connect(context.Background(), "postgresql://gosk:gosk@localhost:5432")
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the database",
			zap.String("Error", err.Error()),
		)
	}
	query := "insert into raw_data (_time, _key, _value) values ($1, $2, $3)"
	m := &nanomsg.RawData{}
	for {
		received, err := socket.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Error on receiving data from the socket",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := proto.Unmarshal(received, m); err != nil {
			logger.GetLogger().Warn(
				"Error on unmarshalling the received data",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if _, err = conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Payload); err != nil {
			logger.GetLogger().Warn(
				"Error on inserting the received data in the database",
				zap.String("Error", err.Error()),
			)
		}
	}
}

// StoreKeyValue saves all received key value messages in the database
func StoreKeyValue(socket mangos.Socket) {
	conn, err := pgx.Connect(context.Background(), "postgresql://gosk:gosk@localhost:5432")
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the database",
			zap.String("Error", err.Error()),
		)
	}
	query := "insert into key_value_data (_time, _key, _context, _path, _value) values ($1, $2, $3, $4, $5)"
	m := &nanomsg.MappedData{}
	for {
		received, err := socket.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Error on receiving data from the socket",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := proto.Unmarshal(received, m); err != nil {
			logger.GetLogger().Warn(
				"Error on unmarshalling the received data",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if m.Datatype == nanomsg.DOUBLE {
			if _, err = conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, m.Path, fmt.Sprintf("%f", m.DoubleValue)); err != nil {
				logger.GetLogger().Warn(
					"Error on inserting the received data in the database",
					zap.String("Error", err.Error()),
				)
			}
		} else if m.Datatype == nanomsg.STRING {
			if _, err = conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, m.Path, m.StringValue); err != nil {
				logger.GetLogger().Warn(
					"Error on inserting the received data in the database",
					zap.String("Error", err.Error()),
				)
			}
		} else if m.Datatype == nanomsg.POSITION {
			if _, err = conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, m.Path, m.PositionValue.String()); err != nil {
				logger.GetLogger().Warn(
					"Error on inserting the received data in the database",
					zap.String("Error", err.Error()),
				)
			}
		} else if m.Datatype == nanomsg.LENGTH {
			if _, err = conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, m.Path, m.LengthValue.String()); err != nil {
				logger.GetLogger().Warn(
					"Error on inserting the received data in the database",
					zap.String("Error", err.Error()),
				)
			}
		}
	}
}
