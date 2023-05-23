package setter

import (
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
)

type ModbusSetter struct {
	config config.SetterConfig
}

func (m *ModbusSetter) Set(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}

// DoSet creates a raw modbus message based on the set message and the setter config
func (m *ModbusSetter) DoSet(*message.Set) (*message.Raw, error) {
	return nil, nil
}
