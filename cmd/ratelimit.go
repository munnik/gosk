package cmd

import (
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var rateLimitCmd = &cobra.Command{
	Use:   "rateLimit",
	Short: "Rate limit incoming data",
	Long:  `Rate limit incoming data at frequencies specified in configuration`,
	Run:   doRateLimit,
}

func init() {
	rootCmd.AddCommand(rateLimitCmd)
	rateLimitCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	rateLimitCmd.MarkFlagRequired("subscribeURL")
	rateLimitCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	rateLimitCmd.MarkFlagRequired("publishURL")
}

func doRateLimit(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	publisher := nanomsg.NewPublisher[message.Mapped](publishURL)
	c := config.NewRateLimitConfig(cfgFile)
	f, _ := mapper.NewRateLimitFilter(c)
	f.Map(subscriber, publisher)
}
