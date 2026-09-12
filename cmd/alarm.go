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

var alarmCmd = &cobra.Command{
	Use:   "alarm",
	Short: "Check incoming mapped data for sane values",
	Long:  `Check incoming mapped data for sane values using expressions specified in configuration, results are published as notifications`,
	Run:   doAlarm,
}

func init() {
	rootCmd.AddCommand(alarmCmd)
	alarmCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	alarmCmd.MarkFlagRequired("subscribeURL")
	alarmCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	alarmCmd.MarkFlagRequired("publishURL")
}

func doAlarm(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	publisher := nanomsg.NewPublisher[message.Mapped](publishURL)
	c := config.NewAlarmMappingConfig(cfgFile)
	m, err := mapper.NewAlarmMapper(c)
	if err != nil {
		logger.GetLogger().Fatal(
			"Error while creating the mapper",
			zap.String("Config file", cfgFile),
			zap.String("Error", err.Error()),
		)
	}
	m.Map(subscriber, publisher)
}
