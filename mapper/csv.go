package mapper

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/antonmedv/expr"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type CsvMapper struct {
	config           config.MapperConfig
	protocol         string
	csvMappingConfig []config.CsvMappingConfig
}

func NewCsvMapper(c config.MapperConfig, cmc []config.CsvMappingConfig) (*CsvMapper, error) {
	return &CsvMapper{config: c, protocol: config.CsvType, csvMappingConfig: cmc}, nil
}

func (m *CsvMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}

func (m *CsvMapper) doMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Collector).WithType(m.protocol)
	u := message.NewUpdate().WithSource(s).WithTimestamp(r.Timestamp)

	for _, cmc := range m.csvMappingConfig {
		stringValue := string(r.Value)
		if !strings.HasPrefix(stringValue, cmc.BeginsWith) {
			continue
		}
		stringValue = stringValue[len(cmc.BeginsWith):]

		// setup env for expression
		stringValues := strings.Split(stringValue, ",")
		floatValues := make([]float64, len(stringValues))
		for i, v := range stringValues {
			if fv, err := strconv.ParseFloat(v, 64); err == nil {
				floatValues[i] = fv
			}
		}
		intValues := make([]int64, len(stringValues))
		for i, v := range stringValues {
			if iv, err := strconv.ParseInt(v, 10, 64); err == nil {
				intValues[i] = iv
			}
		}
		var env = map[string]interface{}{
			"stringValues": stringValues,
			"floatValues":  floatValues,
			"intValues":    intValues,
		}

		if cmc.CompiledExpression == nil {
			// TODO: each iteration the CompiledExpression is nil
			var err error
			if cmc.CompiledExpression, err = expr.Compile(cmc.Expression, expr.Env(env)); err != nil {
				logger.GetLogger().Warn(
					"Could not compile the mapping expression",
					zap.String("Expression", cmc.Expression),
					zap.String("Error", err.Error()),
				)
				continue
			}
		}

		// the compiled program exists, let's run it
		output, err := expr.Run(cmc.CompiledExpression, env)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not run the mapping expression",
				zap.String("Expression", cmc.Expression),
				zap.String("Environment", fmt.Sprintf("%+v", env)),
				zap.String("Error", err.Error()),
			)
			continue
		}
		u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath(cmc.Path).WithValue(output))
	}

	if len(u.Values) == 0 {
		return result, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}
	return result, nil
}
