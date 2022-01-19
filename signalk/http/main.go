package http

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx"
	"github.com/martinlindhe/unit"
	"github.com/munnik/gosk/signalk"
)

// StartServer runs the SignalK HTTP server
func StartServer() {
	conn, err := pgx.Connect(context.Background(), "postgresql://gosk:gosk@localhost:5432")
	if err != nil {
		log.Fatal(err)
	}

	query := "select distinct on(_context, _path) _context, _path, _value, _time from key_value_data order by _context, _path, _time desc"
	var ctx string
	var path string
	var value string
	var timestamp time.Time
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		queryStartTime := time.Now().UnixNano()
		full := signalk.NewFull("test")
		rows, err := conn.Query(context.Background(), query)
		if err != nil {
			log.Fatal(err)
		}
		for rows.Next() {
			err := rows.Scan(&ctx, &path, &value, &timestamp)
			if err != nil {
				log.Fatal(err)
			}
			if len(value) == 0 {
				full.AddValue(signalk.Value{
					Context:   ctx,
					Path:      strings.Split(path, "."),
					Value:     nil,
					Timestamp: timestamp.UTC().Format(time.RFC3339Nano),
				})
			} else if value[0] == byte(34) && value[len(value)-1] == byte(34) { // starts and ends with ", so it's a string
				full.AddValue(signalk.Value{
					Context:   ctx,
					Path:      strings.Split(path, "."),
					Value:     string(value[1 : len(value)-1]),
					Timestamp: timestamp.UTC().Format(time.RFC3339Nano),
				})
			} else if value[0] == byte(123) && value[len(value)-1] == byte(125) { // starts with { and ends with }, so it's a object}
				object, err := signalk.FromJSONToStruct(value, path)
				if err != nil {
					log.Fatal(err)
				}
				full.AddValue(signalk.Value{
					Context:   ctx,
					Path:      strings.Split(path, "."),
					Value:     object,
					Timestamp: timestamp.UTC().Format(time.RFC3339Nano),
				})
			} else { // it must be a number
				floatValue, err := strconv.ParseFloat(value, 64)
				if err != nil {
					log.Fatal("Can't parse as float")
				}
				full.AddValue(signalk.Value{
					Context:   ctx,
					Path:      strings.Split(path, "."),
					Value:     floatValue,
					Timestamp: timestamp.UTC().Format(time.RFC3339Nano),
				})
			}
		}

		json, err := full.MarshalJSON()
		if err != nil {
			log.Fatal(err)
		}

		w.Header().Set("Query-time", fmt.Sprintf("%f seconds", (unit.Duration(time.Now().UnixNano()-queryStartTime)*unit.Nanosecond).Seconds()))
		w.Header().Set("Content-Type", "application/json")
		w.Write(json)
	})

	// listen to port
	http.ListenAndServe(":5050", nil)
}
