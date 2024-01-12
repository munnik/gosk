package cmd

import (
	"encoding/json"
	"fmt"
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

var (
	testCmd = &cobra.Command{
		Use:   "testdata",
		Short: "test data",
		Long:  `generate test data`,
		Run:   doTest,
	}
)

func init() {
	rootCmd.AddCommand(testCmd)
	testCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the collected data on. It listens for connections.")
	testCmd.MarkFlagRequired("publishURL")

}

func doTest(cmd *cobra.Command, args []string) {
	c := config.NewTestDataConfig(cfgFile)
	publisher := nanomsg.NewPub(publishURL)
	defer publisher.Close()
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
			// fmt.Println(result)
			var bytes []byte
			var err error
			if bytes, err = json.Marshal(result); err != nil {
				logger.GetLogger().Warn(
					"Could not marshal the mapped data",
					zap.String("Error", err.Error()),
				)
				continue
			}
			if err := publisher.Send(bytes); err != nil {
				logger.GetLogger().Warn(
					"Unable to send the message using NanoMSG",
					zap.ByteString("Message", bytes),
					zap.String("Error", err.Error()),
				)
				continue
			}
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
