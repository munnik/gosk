package database

import (
	"context"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/jackc/pgconn"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
)

type pgxExecInterface interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}

// StoreRaw saves all received raw messages in the database
func StoreRaw(bytesChannel <-chan []byte, db pgxExecInterface) {
	query := `INSERT INTO raw_data ("time", "key", "value") VALUES ($1, $2, $3)`
	m := &nanomsg.RawData{}
	for received := range bytesChannel {
		if err := proto.Unmarshal(received, m); err != nil {
			logger.GetLogger().Warn(
				"Error on unmarshalling the received data",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if _, err := db.Exec(context.Background(), query, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Payload); err != nil {
			logger.GetLogger().Warn(
				"Error on inserting the received data in the database",
				zap.String("Error", err.Error()),
			)
		}
	}
	logger.GetLogger().Warn(
		"Channel is closed, returning from StoreRaw",
	)
}

// StoreKeyValue saves all received key value messages in the database
func StoreKeyValue(bytesChannel <-chan []byte, db pgxExecInterface) {
	query_double := `INSERT INTO key_value_data ("time", "key", "context", "path", "value_double") VALUES ($1, $2, $3, $4, $5)`
	query_text := `INSERT INTO key_value_data ("time", "key", "context", "path", "value_text") VALUES ($1, $2, $3, $4, $5)`
	m := &nanomsg.MappedData{}
	for received := range bytesChannel {
		if err := proto.Unmarshal(received, m); err != nil {
			logger.GetLogger().Warn(
				"Error on unmarshalling the received data",
				zap.String("Error", err.Error()),
			)
			continue
		}

		switch v := m.Value.(type) {
		case *nanomsg.MappedData_StringValue:
			if _, err := db.Exec(context.Background(), query_text, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, m.Path, v.StringValue); err != nil {
				logger.GetLogger().Warn(
					"Error on inserting the received data in the database",
					zap.String("Error", err.Error()),
				)
			}
		case *nanomsg.MappedData_DoubleValue:
			if _, err := db.Exec(context.Background(), query_double, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, m.Path, v.DoubleValue); err != nil {
				logger.GetLogger().Warn(
					"Error on inserting the received data in the database",
					zap.String("Error", err.Error()),
				)
			}
		case *nanomsg.MappedData_PositionValue:
			if v.PositionValue.Altitude != nil {
				if _, err := db.Exec(context.Background(), query_double, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, strings.TrimLeft(m.Path+".altitude", "."), v.PositionValue.Altitude); err != nil {
					logger.GetLogger().Warn(
						"Error on inserting the received data in the database",
						zap.String("Error", err.Error()),
					)
				}
			}
			if v.PositionValue.Latitude != nil {
				if _, err := db.Exec(context.Background(), query_double, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, strings.TrimLeft(m.Path+".latitude", "."), v.PositionValue.Latitude); err != nil {
					logger.GetLogger().Warn(
						"Error on inserting the received data in the database",
						zap.String("Error", err.Error()),
					)
				}
			}
			if v.PositionValue.Longitude != nil {
				if _, err := db.Exec(context.Background(), query_double, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, strings.TrimLeft(m.Path+".longitude", "."), v.PositionValue.Longitude); err != nil {
					logger.GetLogger().Warn(
						"Error on inserting the received data in the database",
						zap.String("Error", err.Error()),
					)
				}
			}
		case *nanomsg.MappedData_LengthValue:
			if v.LengthValue.Hull != nil {
				if _, err := db.Exec(context.Background(), query_double, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, strings.TrimLeft(m.Path+".hull", "."), v.LengthValue.Hull); err != nil {
					logger.GetLogger().Warn(
						"Error on inserting the received data in the database",
						zap.String("Error", err.Error()),
					)
				}
			}
			if v.LengthValue.Overall != nil {
				if _, err := db.Exec(context.Background(), query_double, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, strings.TrimLeft(m.Path+".overall", "."), v.LengthValue.Overall); err != nil {
					logger.GetLogger().Warn(
						"Error on inserting the received data in the database",
						zap.String("Error", err.Error()),
					)
				}
			}
			if v.LengthValue.Waterline != nil {
				if _, err := db.Exec(context.Background(), query_double, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, strings.TrimLeft(m.Path+".waterline", "."), v.LengthValue.Waterline); err != nil {
					logger.GetLogger().Warn(
						"Error on inserting the received data in the database",
						zap.String("Error", err.Error()),
					)
				}
			}
		case *nanomsg.MappedData_VesselDataValue:
			if v.VesselDataValue.Mmsi != nil {
				if _, err := db.Exec(context.Background(), query_text, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, strings.TrimLeft(m.Path+".mmsi", "."), v.VesselDataValue.Mmsi); err != nil {
					logger.GetLogger().Warn(
						"Error on inserting the received data in the database",
						zap.String("Error", err.Error()),
					)
				}
			}
			if v.VesselDataValue.Name != nil {
				if _, err := db.Exec(context.Background(), query_text, m.Timestamp.AsTime(), m.Header.HeaderSegments, m.Context, strings.TrimLeft(m.Path+".name", "."), v.VesselDataValue.Name); err != nil {
					logger.GetLogger().Warn(
						"Error on inserting the received data in the database",
						zap.String("Error", err.Error()),
					)
				}
			}
		}
	}
	logger.GetLogger().Warn(
		"Channel is closed, returning from StoreRaw",
	)
}
