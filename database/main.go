package database

import (
	"fmt"
	"time"

	"github.com/munnik/gosk/message"
)

type DatabaseWriter interface {
	WriteRaw(raw *message.Raw)
	WriteMapped(mapped *message.Mapped)
}

type DatabaseReader interface {
	ReadRaw(where fmt.Stringer) ([]message.Raw, error)
	ReadMapped(where fmt.Stringer) ([]message.Mapped, error)
}

type WhereClause interface {
	Arguments() []interface{}
	String() string
}

type IntervalWhereClause struct {
	parameter string
	from      time.Time
	to        time.Time
}

func NewIntervalWhereClause(parameter string, from, to time.Time) *IntervalWhereClause {
	return &IntervalWhereClause{parameter: parameter, from: from, to: to}
}

func (wc *IntervalWhereClause) Arguments() []time.Time {
	return []time.Time{wc.from, wc.to}
}

func (wc *IntervalWhereClause) String() string {
	return "WHERE " + wc.parameter + " BETWEEN $1 AND $2"
}
