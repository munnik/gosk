package transfer

import (
	"time"
)

const (
	QueryCmd   = "query"
	RequestCmd = "request"
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
