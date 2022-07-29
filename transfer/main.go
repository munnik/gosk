package transfer

import "github.com/munnik/gosk/message"

const (
	QueryCmd   = "query"
	RequestCmd = "request"
	Epoch      = "2022-01-01T00:00:00.000Z"
)

type CommandMessage struct {
	Command string
	Request message.TransferMessage
}
