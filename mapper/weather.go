package mapper

import (
	"time"

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

// GetTickerInterval returns the interval on which the mapper should
// additionally be re-evaluated regardless of incoming data, see
// periodicMapper in main.go. Zero disables this.
func (m *WeatherMapper) GetTickerInterval() time.Duration {
	return m.config.Interval
}

func (m *WeatherMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

func (m *WeatherMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	return input, nil
}
