package writer

import (
	"go.nanomsg.org/mangos/v3"
)

// RawWriter interface
type RawWriter interface {
	WriteRaw(subscriber mangos.Socket)
}

// RawWriter interface
type MappedWriter interface {
	WriteMapped(subscriber mangos.Socket)
}
