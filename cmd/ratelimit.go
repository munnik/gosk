package cmd

import (
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/ratelimit"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	rateLimitCmd = &cobra.Command{
		Use:   "rateLimit",
		Short: "Rate limit incoming data",
		Long:  `Rate limit incoming data at frequencies specified in configuration`,
		Run:   doLimit,
	}
)

func init() {
	rootCmd.AddCommand(rateLimitCmd)
	rateLimitCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	rateLimitCmd.MarkFlagRequired("subscribeURL")
	rateLimitCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	rateLimitCmd.MarkFlagRequired("publishURL")
}

func doLimit(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	publisher := nanomsg.NewPub(publishURL)
	m, _ := ratelimit.NewMappedRateLimiter()
	m.RateLimit(subscriber, publisher)
}
