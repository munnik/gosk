package reader

import (
	"go.nanomsg.org/mangos/v3"
)

type MappedReader interface {
	ReadMapped(publisher mangos.Socket)
}
