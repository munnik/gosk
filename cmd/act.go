package cmd

import (
	"github.com/munnik/gosk/actor"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var actCmd = &cobra.Command{
	Use:   "act",
	Short: "Act on mapped data",
	Long:  `Take actions on incoming mapped data`,
	Run:   doAct,
}

func init() {
	rootCmd.AddCommand(actCmd)
	actCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	actCmd.MarkFlagRequired("subscribeURL")
	actCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	actCmd.MarkFlagRequired("publishURL")
}

func doAct(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	publisher := nanomsg.NewPub(publishURL)

	c := config.NewActorConfig(cfgFile)
	var a actor.Actor
	switch c.Protocol {
	case config.ModbusType:
		a, err = actor.NewModbusActor(c)

	default:
		logger.GetLogger().Fatal(
			"Not a supported protocol",
			zap.String("Protocol", c.Protocol),
		)
		return
	}
	if err != nil {
		logger.GetLogger().Fatal(
			"Error while creating the actor",
			zap.String("Config file", cfgFile),
			zap.String("Error", err.Error()),
		)
	}
	a.Act(subscriber, publisher)
}
