package mapper

import (
	"encoding/binary"
	"fmt"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/mapper/dbc"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"

	"github.com/brutella/can"
)

type CanBusMapper struct {
	config   config.MapperConfig
	protocol string
	dbc      dbc.DBC
}

func NewCanBusMapper(c config.MapperConfig) (*CanBusMapper, error) {
	// parse DBC file and store mappings
	dbc := dbc.NewDBC("/home/albert/Documents/FuelEssence/TelMA_ID0x100.dbc")
	// fmt.Print(dbc)
	return &CanBusMapper{config: c, protocol: config.CanBusType, dbc: dbc}, nil
}

func (m *CanBusMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}
func (m *CanBusMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Collector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)

	frm := createFrame(r)
	//lookup mappings for frame
	mappings, present := m.dbc[frm.ID]
	if present {
		// apply all mappings
		for _, mapping := range mappings.Signals {
			// get name
			name := mapping.Name
			// get value
			val := 1.0
			start := mapping.StartBit
			length := mapping.Size
			data := make([]uint8, 8)
			copy(data, frm.Data[:])
			if mapping.IsBigEndian {
				start = start - 7
				// reverse the bits in each byte
				// for i, b := range data {
				// 	data[i] = bits.Reverse8(b)
				// }
			}
			// fmt.Println(data)

			temp := binary.BigEndian.Uint64(data[:])
			temp = temp << start
			temp = temp >> (64 - (length))
			// fmt.Printf("%x\n", temp)
			if mapping.IsSigned {
				val = float64(int64(temp))
			} else {
				val = float64(temp)
			}

			// get conversion
			res := val*mapping.Factor + mapping.Offset

			fmt.Printf("[%s %f]\n", name, res)
		}
	}

	// fmt.Println(frm)
	return result.AddUpdate(u), nil
}

func createFrame(r *message.Raw) can.Frame {
	data := [8]uint8{}
	copy(data[:], r.Value[8:16])
	frm := can.Frame{
		ID:     binary.BigEndian.Uint32(r.Value[0:4]),
		Length: r.Value[4],
		Flags:  r.Value[5],
		Res0:   r.Value[6],
		Res1:   r.Value[7],
		Data:   data,
	}
	return frm
}
