package transfer

import (
	"time"

	"github.com/google/uuid"
)

const (
	requestCountCmd      = "count"
	requestDataCmd       = "data"
	requestTopic         = "request/%s"
	respondTopic         = "respond/%s"
	periodDuration       = 5 * time.Minute
	countRequestCoolDown = 12 * periodDuration // only send count request for periods up to one hour ago
)

type RequestMessage struct {
	Command       string            `json:"command"`
	PeriodStart   time.Time         `json:"period_start"`
	UUID          uuid.UUID         `json:"uuid"`
	CountsPerUuid map[uuid.UUID]int `json:"counts_per_uuid,omitempty"` // map of raw_data uuids we got mapped_data for, the number is the number of data points we already have per raw_data uuid
}

type ResponseMessage struct {
	DataPoints  int       `json:"data_points"`
	PeriodStart time.Time `json:"period_start"`
}
