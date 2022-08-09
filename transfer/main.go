package transfer

import (
	"github.com/munnik/gosk/message"
)

const (
	requestCountCmd            = "count"
	requestDataCmd             = "data"
	requestTopic               = "request/%s"
	respondTopic               = "respond/%s"
	betweenIntervalWhereClause = `WHERE "origin" = $1 AND "time" BETWEEN $2 AND $3;`
	forOriginWhereClause       = `WHERE "origin" = $1 ORDER BY "start"`
)

type CommandMessage struct {
	Command string
	Request message.TransferRequest
}
