package transfer

import (
	"time"
)

const (
	QueryCmd   = "query"
	RequestCmd = "request"
	Epoch      = "2022-01-01T00:00:00.000Z"
)

type TransferMessage struct {
	Origin           string
	PeriodStart      time.Time
	PeriodEnd        time.Time
	LocalDataPoints  int
	RemoteDataPoints int
}
type CommandMessage struct {
	Command string
	Request TransferMessage
}
