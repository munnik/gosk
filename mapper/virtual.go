package mapper

import (
	"github.com/munnik/gosk/config"
	"go.nanomsg.org/mangos/v3"
)

type VirtualMapper struct {
	config config.MapperConfig
}

func NewVirtualMapper(c config.MapperConfig) (*VirtualMapper, error) {
	return &VirtualMapper{config: c}, nil
}

func (m *VirtualMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	// incoming messages are already mapped, this mapper combines several mapped data points to a new 'virtual' mapped data point
}
