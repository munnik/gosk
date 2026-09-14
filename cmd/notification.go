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

var notificationCmd = &cobra.Command{
	Use:   "notification",
	Short: "Check incoming mapped data and raise Signal K notifications",
	Long:  `Check incoming mapped data using expressions specified in configuration and publish the results as Signal K notifications`,
	Run:   doNotification,
}

func init() {
	rootCmd.AddCommand(notificationCmd)
	notificationCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	notificationCmd.MarkFlagRequired("subscribeURL")
	notificationCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	notificationCmd.MarkFlagRequired("publishURL")
}

func doNotification(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	publisher := nanomsg.NewPublisher[message.Mapped](publishURL)
	c := config.NewMapperConfig(cfgFile)
	checks := config.NewNotificationMappingConfig(cfgFile)
	m, err := mapper.NewNotificationMapper(c, checks)
	if err != nil {
		logger.GetLogger().Fatal(
			"Error while creating the mapper",
			zap.String("Config file", cfgFile),
			zap.String("Error", err.Error()),
		)
	}
	m.Map(subscriber, publisher)
}
