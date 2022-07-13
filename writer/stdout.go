package writer

import (
	"fmt"

	"github.com/munnik/gosk/logger"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type StdOutWriter struct {
}

func NewStdOutWriter() *StdOutWriter {
	return &StdOutWriter{}
}

func (w *StdOutWriter) WriteMapped(subscriber mangos.Socket) {
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		fmt.Println(received)
	}
}

func (w *StdOutWriter) WriteRaw(subscriber mangos.Socket) {
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		fmt.Println(received)
	}
}
