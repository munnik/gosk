package transfer

import (
	"time"

	"github.com/munnik/gosk/message"
)

const (
	requestCountCmd                = "requestCount"
	requestDataCmd                 = "requestData"
	countData                      = "requestCount/%s"
	replyTopic                     = "respondCount/%s"
	betweenIntervalWhereClause     = `WHERE "origin" = $1 AND "time" BETWEEN $2 AND $3;`
	localMoreThanRemoteWhereClause = `WHERE "origin" = $1 ORDER BY "start"`
)

var (
	Epoch = time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
)

type CommandMessage struct {
	Command string
	Request message.TransferRequest
}
