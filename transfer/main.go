package transfer

import (
	"github.com/munnik/gosk/message"
)

const (
	requestCountCmd            = "requestCount"
	requestDataCmd             = "requestData"
	countData                  = "requestCount/%s"
	replyTopic                 = "respondCount/%s"
	betweenIntervalWhereClause = `WHERE "origin" = $1 AND "time" BETWEEN $2 AND $3;`
	forOriginWhereClause       = `WHERE "origin" = $1 ORDER BY "start"`
)

type CommandMessage struct {
	Command string
	Request message.TransferRequest
}
