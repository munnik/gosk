package database

import (
	"context"
	"strings"

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

		switch v := m.Value.(type) {
		case *nanomsg.MappedData_StringValue, *nanomsg.MappedData_DoubleValue:
			insert(conn, query, m, m.Path, v)
		case *nanomsg.MappedData_PositionValue:
			if v.PositionValue.Altitude != nil {
				insert(conn, query, m, strings.TrimLeft(m.Path+".altitude", "."), v.PositionValue.Altitude)
			}
			if v.PositionValue.Latitude != nil {
				insert(conn, query, m, strings.TrimLeft(m.Path+".latitude", "."), v.PositionValue.Latitude)
			}
			if v.PositionValue.Longitude != nil {
				insert(conn, query, m, strings.TrimLeft(m.Path+".longitude", "."), v.PositionValue.Longitude)
			}
		case *nanomsg.MappedData_LengthValue:
			if v.LengthValue.Hull != nil {
				insert(conn, query, m, strings.TrimLeft(m.Path+".hull", "."), v.LengthValue.Hull)
			}
			if v.LengthValue.Overall != nil {
				insert(conn, query, m, strings.TrimLeft(m.Path+".overall", "."), v.LengthValue.Overall)
			}
			if v.LengthValue.Waterline != nil {
				insert(conn, query, m, strings.TrimLeft(m.Path+".waterline", "."), v.LengthValue.Waterline)
			}
		case *nanomsg.MappedData_VesselDataValue:
			if v.VesselDataValue.Mmsi != nil {
				insert(conn, query, m, strings.TrimLeft(m.Path+".mmsi", "."), v.VesselDataValue.Mmsi)
			}
			if v.VesselDataValue.Name != nil {
				insert(conn, query, m, strings.TrimLeft(m.Path+".name", "."), v.VesselDataValue.Name)
			}
		}
	}
}

func insert(conn *pgx.Conn, query string, m *nanomsg.MappedData, path string, value interface{}) {
	execQuery := func(stringValue string) {
		if _, err := conn.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, path, stringValue); err != nil {
			logger.GetLogger().Warn(
				"Error on inserting the received data in the database",
				zap.String("Error", err.Error()),
			)
		}
	}

	stringValue, err := nanomsg.ToString(value)
	if err != nil {
		logger.GetLogger().Warn(
			"Can't insert unsupported type in database",
			zap.String("Error", err.Error()),
		)
		return
	}

	execQuery(stringValue)
}
