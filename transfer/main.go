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

// LoggedRequestMessage is what goes into transfer_log in place of a
// RequestMessage: the same fields, but with CountsPerUuid reduced to its
// size.
//
// The map holds one entry per outstanding raw_data uuid, so a vessel with a
// backlog produced log rows of ~344kB (377kB seen) - 0.8% of the rows in
// transfer_log carrying ~96% of its bytes. That made transfer_log 18TB, 62%
// of the whole database, and 15.3TB of it was written in the seven months
// after the fleet grew. The uuids are not worth keeping: every one of them
// is already in mapped_data.uuid. How far behind the vessel was is worth
// keeping, and that is what uuid_count records.
type LoggedRequestMessage struct {
	Command     string    `json:"command"`
	UUID        uuid.UUID `json:"uuid"`
	PeriodStart time.Time `json:"period_start"`
	UuidCount   int       `json:"uuid_count"`
}

// ForLog returns the form of this message that is written to transfer_log.
func (m RequestMessage) ForLog() LoggedRequestMessage {
	return LoggedRequestMessage{
		Command:     m.Command,
		UUID:        m.UUID,
		PeriodStart: m.PeriodStart,
		UuidCount:   len(m.CountsPerUuid),
	}
}
