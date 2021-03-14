package collector

import (
	"go.nanomsg.org/mangos/v3"
)

// Collector interface
type Collector interface {
	Collect(mangos.Socket)
}
