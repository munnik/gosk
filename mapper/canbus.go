package mapper

import (
	"fmt"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
)

type CanBusMapper struct {
	config   config.MapperConfig
	protocol string
}

func NewCanBusMapper(c config.MapperConfig) (*CanBusMapper, error) {
	return &CanBusMapper{config: c, protocol: config.CanBusType}, nil
}

func (m *CanBusMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}
func (m *CanBusMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Collector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)
	fmt.Println(r)
	return result.AddUpdate(u), nil
}
