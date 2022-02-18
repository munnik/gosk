package mapper

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
)

type SygoMapper struct {
	config   config.MapperConfig
	protocol string
}

func NewSygoMapper(c config.MapperConfig) (*SygoMapper, error) {
	return &SygoMapper{config: c, protocol: config.SygoType}, nil
}

func (m *SygoMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}

func (m *SygoMapper) doMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Collector).WithType(m.protocol)
	u := message.NewUpdate().WithSource(s).WithTimestamp(r.Timestamp)

	// key : units : description
	// 01 : cm : draft portside forward
	// 02 : cm : draft starboard forward
	// 03 : cm : draft portside center
	// 04 : cm : draft starboard center
	// 05 : cm : draft portside aft
	// 06 : cm : draft starboard aft
	// G : cm : current average draft
	// T : Mg : tonnage
	columns := strings.SplitN(string(r.Value), ",", 2)
	switch columns[0] {
	case "01":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath("design.draft.portside.forward").WithValue(n / 100.0))
		}
	case "02":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath("design.draft.starboard.forward").WithValue(n / 100.0))
		}
	case "03":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath("design.draft.portside.center").WithValue(n / 100.0))
		}
	case "04":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath("design.draft.starboard.center").WithValue(n / 100.0))
		}
	case "05":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath("design.draft.portside.aft").WithValue(n / 100.0))
		}
	case "06":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath("design.draft.starboard.aft").WithValue(n / 100.0))
		}
	case "G":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath("design.draft.current").WithValue(n / 100.0))
		}
	case "T":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath("design.displacement").WithValue(n * 1000.0))
		}
	}

	if len(u.Values) == 0 {
		return result, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}
	return result, nil
}
