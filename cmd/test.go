package cmd

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
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
	publisher := nanomsg.NewPub(publishURL)
	defer publisher.Close()
	ticker := time.NewTicker(1 * time.Millisecond)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		i := 0
		for range ticker.C {

			result := message.NewMapped().WithContext("m.config.Context").WithOrigin("m.config.Context")
			s := message.NewSource().WithLabel("sampleData").WithType("sampleData").WithUuid(uuid.New())
			u := message.NewUpdate().WithSource(*s).WithTimestamp(time.Now())
			u.AddValue(message.NewValue().WithPath("mmc.Path").WithValue(i))
			// u.AddValue(message.NewValue().WithPath("mmc.Path2").WithValue(i))
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
