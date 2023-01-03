package transfer

import (
	"time"

	"github.com/google/uuid"
)

const (
	requestCountCmd = "count"
	requestDataCmd  = "data"
	requestTopic    = "request/%s"
	respondTopic    = "respond/%s"
	periodDuration  = 5 * time.Minute
)

type RequestMessage struct {
	Command     string    `json:"command"`
	PeriodStart time.Time `json:"period_start"`
	UUID        uuid.UUID `json:"uuid"`
}

type ResponseMessage struct {
	DataPoints  int       `json:"data_points"`
	PeriodStart time.Time `json:"period_start"`
	UUID        uuid.UUID `json:"uuid"`
}
