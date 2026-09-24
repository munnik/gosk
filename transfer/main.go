package transfer

import (
	"time"

	"github.com/munnik/uuid/v5"
)

const (
	countCmd             = "count"
	dataCmd              = "data"
	requestTopic         = "request/%s"
	respondTopic         = "respond/%s"
	periodDuration       = 5 * time.Minute
	countRequestCoolDown = 12 * periodDuration // only send count request for periods up to one hour ago

	// transferQoS is 1. The mechanism that repairs data lost to a flaky
	// link was itself sent at QoS 0 over that same link, so a request or a
	// response could disappear exactly as silently as the data it was
	// meant to recover - and then be re-sent whole cycles later.
	transferQoS = 1
	// transferRetained is false. Retaining these topics was actively
	// harmful: every request published to request/<origin> replaced the
	// previous one, so a responder that reconnected was handed one
	// arbitrary stale request (never the backlog) and answered it again,
	// and a requester subscribing to respond/# re-processed the last
	// response of every origin on every reconnect.
	transferRetained = false
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
