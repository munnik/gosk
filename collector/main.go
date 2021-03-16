package collector

import (
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

// Collector interface
type Collector interface {
	Collect(mangos.Socket)
}

var Logger *zap.Logger
