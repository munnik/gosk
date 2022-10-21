package reader

import (
	"go.nanomsg.org/mangos/v3"
)

type RawReader interface {
	ReadRaw(publisher mangos.Socket)
}
type MappedReader interface {
	ReadMapped(publisher mangos.Socket)
}
