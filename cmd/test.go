package cmd

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const bufferCapacity = 5000

var (
	testCmd = &cobra.Command{
		Use:   "testdata",
		Short: "test data",
		Long:  `generate test data`,
	}
	testMappedCmd = &cobra.Command{
		Use:   "mapped",
		Short: "mapped test data",
		Long:  `generate mapped test data`,
		Run:   doTest,
	}
	testRawCmd = &cobra.Command{
		Use:   "raw",
		Short: "raw test data",
		Long:  `generate raw test data`,
		Run:   doRawTest,
	}
)

func init() {
	rootCmd.AddCommand(testCmd)
	testCmd.PersistentFlags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the collected data on. It listens for connections.")
	testCmd.MarkFlagRequired("publishURL")

	testCmd.AddCommand(testMappedCmd)
	testCmd.AddCommand(testRawCmd)
}

func doTest(cmd *cobra.Command, args []string) {
	sendBuffer := make(chan *message.Mapped, bufferCapacity)
	defer close(sendBuffer)
	publisher := nanomsg.NewPublisher[message.Mapped](publishURL)
	go publisher.Send(sendBuffer)

	c := config.NewTestDataConfig(cfgFile)
	ticker := time.NewTicker(c.Delay)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		i := 0
		for range ticker.C {
			result := message.NewMapped().WithContext(c.Context).WithOrigin(c.Context)
			s := message.NewSource().WithLabel("sampleData").WithType("sampleData").WithUuid(uuid.New())
			u := message.NewUpdate().WithSource(*s).WithTimestamp(time.Now())
			for _, path := range c.Paths {
				vm := vm.VM{}
				env := make(map[string]interface{})
				env["value"] = i
				output, err := runExpr(vm, env, path)
				if err == nil {
					u.AddValue(message.NewValue().WithPath(path.Path).WithValue(output))
				} else {
					logger.GetLogger().Error(
						"Could not map value",
						zap.String("path", path.Path),
						zap.String("error", err.Error()),
					)
				}

			}

			result.AddUpdate(u)
			i++
			sendBuffer <- result
		}
	}()
	wg.Wait()
}

func doRawTest(cmd *cobra.Command, args []string) {
	sendBuffer := make(chan *message.Raw, bufferCapacity)
	defer close(sendBuffer)
	publisher := nanomsg.NewPublisher[message.Raw](publishURL)
	go publisher.Send(sendBuffer)

	c := config.NewTestDataConfig(cfgFile)
	ticker := time.NewTicker(c.Delay)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		i := 0
		for range ticker.C {
			for _, path := range c.Paths {
				result := message.NewRaw().WithConnector("sampleData").WithType("sample")

				vm := vm.VM{}
				env := make(map[string]interface{})
				env["value"] = i
				output, err := runExpr(vm, env, path)
				if err == nil {
					array, ok := output.([]interface{})
					if !ok {
						logger.GetLogger().Error("expression should return an array of register values")
					}
					registers := make([]int, len(array))
					for i, v := range array {
						value := v.(int)
						registers[i] = value
					}
					bytes := make([]byte, 0, len(array)*2)
					for _, v := range registers {
						if v > math.MaxUint16 || v < 0 {
							logger.GetLogger().Error("register value out of range.", zap.Int("value", v))
						} else {
							uv := uint16(v)
							bytes = binary.BigEndian.AppendUint16(bytes, uv)
						}
					}
					result.WithValue(bytes)
				} else {
					logger.GetLogger().Error(
						"Could not parse value",
						zap.String("path", path.Path),
						zap.String("error", err.Error()),
					)
				}
				sendBuffer <- result
			}

			i++
		}
	}()
	wg.Wait()
}
func runExpr(vm vm.VM, env map[string]interface{}, mappingConfig config.MappingConfig) (interface{}, error) {
	if mappingConfig.CompiledExpression == nil {
		// TODO: each iteration the CompiledExpression is nil
		var err error
		if mappingConfig.CompiledExpression, err = expr.Compile(mappingConfig.Expression, expr.Env(env)); err != nil {
			logger.GetLogger().Warn(
				"Could not compile the mapping expression",
				zap.String("Expression", mappingConfig.Expression),
				zap.String("Error", err.Error()),
			)
			return nil, err
		}
	}
	// the compiled program exists, let's run it
	output, err := vm.Run(mappingConfig.CompiledExpression, env)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not run the mapping expression",
			zap.String("Expression", mappingConfig.Expression),
			zap.String("Environment", fmt.Sprintf("%+v", env)),
			zap.String("Error", err.Error()),
		)
		return nil, err
	}

	// the value is a map so we could try to decode it
	if m, ok := output.(map[string]interface{}); ok {
		if decoded, err := message.Decode(m); err == nil {
			output = decoded
		}
	}

	return output, nil
}
