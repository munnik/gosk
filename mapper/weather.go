package mapper

import (
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

type WeatherMapper struct {
	config config.MapperConfig
}

func NewWeatherMapper(c config.MapperConfig, emc []*config.ExpressionMappingConfig) (*WeatherMapper, error) {
	return &WeatherMapper{config: c}, nil
}

func (m *WeatherMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

func (m *WeatherMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	return input, nil
}
