package message

import "time"

type TransferMessage struct {
	Origin           string
	PeriodStart      time.Time
	PeriodEnd        time.Time
	LocalDataPoints  int
	RemoteDataPoints int
}
