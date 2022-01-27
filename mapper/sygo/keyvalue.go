package sygo

import (
	"strconv"
	"strings"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper/signalk"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

func KeyValueFromSygo(m *nanomsg.RawData, cfg config.SygoConfig) ([]signalk.Value, error) {
	result := make([]signalk.Value, 0)
	logger.GetLogger().Warn(
		"Mapper got data",
		zap.String("String", string(m.Payload)),
		zap.ByteString("Bytes", m.Payload),
	)
	context := cfg.Context
	// key : units : description
	// 01 : cm : draft portside forward
	// 02 : cm : draft starboard forward
	// 03 : cm : draft portside center
	// 04 : cm : draft starboard center
	// 05 : cm : draft portside aft
	// 06 : cm : draft starboard aft
	// G : cm : current average draft
	// T : Mg : tonnage
	columns := strings.SplitN(string(m.Payload), ",", 2)
	switch columns[0] {
	case "01":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "draft", "portside", "forward"}, Value: nanomsg.Double(n / 100.0)})
		}
	case "02":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "draft", "starboard", "forward"}, Value: nanomsg.Double(n / 100.0)})
		}
	case "03":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "draft", "portside", "center"}, Value: nanomsg.Double(n / 100.0)})
		}
	case "04":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "draft", "starboard", "center"}, Value: nanomsg.Double(n / 100.0)})
		}
	case "05":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "draft", "portside", "aft"}, Value: nanomsg.Double(n / 100.0)})
		}
	case "06":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "draft", "starboard", "aft"}, Value: nanomsg.Double(n / 100.0)})
		}
	case "G":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "draft", "current"}, Value: nanomsg.Double(n / 100.0)})
		}
	case "T":
		if n, err := strconv.ParseFloat(strings.TrimSpace(columns[1]), 64); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "displacement"}, Value: nanomsg.Double(n * 1000.0)})
		}
	}

	return result, nil
}
