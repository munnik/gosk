package cmd

import (
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/nanomsg"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var filterCmd = &cobra.Command{
	Use:   "filter",
	Short: "Filter incoming data",
	Long:  `Filter incoming data for unwanted sources, values, etc using an expression`,
	Run:   doFilter,
}

func init() {
	rootCmd.AddCommand(filterCmd)
	filterCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	filterCmd.MarkFlagRequired("subscribeURL")
	filterCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	filterCmd.MarkFlagRequired("publishURL")
}

func doFilter(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	publisher := nanomsg.NewPub(publishURL)
	c := config.NewExpressionMappingConfig(cfgFile)
	f, _ := mapper.NewExpressionFilter(c)
	f.Map(subscriber, publisher)
}
