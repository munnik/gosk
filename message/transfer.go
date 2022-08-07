package message

import "time"

type TransferRequest struct {
	Origin           string
	PeriodStart      time.Time
	PeriodEnd        time.Time
	LocalDataPoints  int
	RemoteDataPoints int
}
