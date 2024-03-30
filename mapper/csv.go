package mapper

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

type CSVMapper struct {
	config           config.CSVMapperConfig
	protocol         string
	csvMappingConfig []config.CSVMappingConfig
}

func NewCSVMapper(c config.CSVMapperConfig, cmc []config.CSVMappingConfig) (*CSVMapper, error) {
	return &CSVMapper{config: c, protocol: config.CSVType, csvMappingConfig: cmc}, nil
}

func (m *CSVMapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

func (m *CSVMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)

	env := NewExpressionEnvironment()

	for _, cmc := range m.csvMappingConfig {
		stringInput := string(r.Value)
		lines := make([]string, 0)
		if m.config.SplitLines {
			sc := bufio.NewScanner(strings.NewReader(stringInput))
			for sc.Scan() {
				lines = append(lines, sc.Text())
			}
		} else {
			lines = append(lines, stringInput)
		}

		for _, stringValue := range lines {
			if !strings.HasPrefix(stringValue, cmc.BeginsWith) {
				continue
			}
			stringValue = stringValue[len(cmc.BeginsWith):]
			var r *regexp.Regexp
			if cmc.Regex != "" {
				var err error
				if r, err = regexp.Compile(cmc.Regex); err != nil {
					logger.GetLogger().Warn(
						"Could not compile the regular expression",
						zap.String("Regex", cmc.Regex),
						zap.String("Error", err.Error()),
					)
					continue
				}
			}
			// setup env for expression
			stringValues := strings.Split(stringValue, m.config.Separator)
			floatValues := make([]float64, len(stringValues))
			intValues := make([]int64, len(stringValues))
			for i := range stringValues {
				if r != nil {
					stringValues[i] = r.ReplaceAllString(stringValues[i], cmc.ReplaceWith)
				}
				if fv, err := strconv.ParseFloat(stringValues[i], 64); err == nil {
					floatValues[i] = fv
				} else if fv, err := strconv.ParseFloat(swapPointAndComma(stringValues[i]), 64); err == nil {
					floatValues[i] = fv
				}
				if iv, err := strconv.ParseInt(stringValues[i], 10, 64); err == nil {
					intValues[i] = iv
				}
			}

			env["stringValues"] = stringValues
			env["floatValues"] = floatValues
			env["intValues"] = intValues

			output, err := runExpr(env, &cmc.MappingConfig)
			if err == nil {
				u.AddValue(message.NewValue().WithPath(cmc.Path).WithValue(output))
			}
		}
	}

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result.AddUpdate(u), nil
}
