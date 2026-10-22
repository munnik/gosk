package transfer

import (
	"time"

	"github.com/google/uuid"
)

const (
	countCmd             = "count"
	dataCmd              = "data"
	requestTopic         = "request/%s"
	respondTopic         = "respond/%s"
	periodDuration       = 5 * time.Minute
	countRequestCoolDown = 12 * periodDuration // only send count request for periods up to one hour ago
)

type RequestMessage struct {
	Command       string            `json:"command"`
	UUID          uuid.UUID         `json:"uuid"`
	PeriodStart   time.Time         `json:"period_start"`
	CountsPerUuid map[uuid.UUID]int `json:"counts_per_uuid,omitempty"` // map of raw_data uuids we got mapped_data for, the number is the number of data points we already have per raw_data uuid
}

type ResponseMessage struct {
	Command     string    `json:"command"`
	UUID        uuid.UUID `json:"uuid"`
	PeriodStart time.Time `json:"period_start"`
	DataPoints  int       `json:"data_points"`
}
