package cmd

import (
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/nanomsg"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Map raw data to meaningful data",
	Long:  `Map raw data to meaningful data based on the SignalK specification`,
	Run:   doMap,
}

func init() {
	rootCmd.AddCommand(mapCmd)
	mapCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	mapCmd.MarkFlagRequired("subscribeURL")
	mapCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	mapCmd.MarkFlagRequired("publishURL")
}

func doMap(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	publisher := nanomsg.NewPub(publishURL)

	c := config.NewMapperConfig(cfgFile)
	var m mapper.Mapper
	switch c.Protocol {
	case config.CSVType:
		c2 := config.NewCSVMapperConfig(cfgFile)
		cmc := config.NewCSVMappingConfig(cfgFile)
		m, err = mapper.NewCSVMapper(c2, cmc)
	case config.JSONType:
		jmc := config.NewJSONMappingConfig(cfgFile)
		m, err = mapper.NewJSONMapper(c, jmc)
	case config.ModbusType:
		rmc := config.NewModbusMappingsConfig(cfgFile)
		m, err = mapper.NewModbusMapper(c, rmc)
	case config.NMEA0183Type:
		m, err = mapper.NewNmea0183Mapper(c)
	case config.CanBusType:
		c2 := config.NewCanBusMapperConfig(cfgFile)
		cmc := config.NewCanBusMappingConfig(cfgFile)
		m, err = mapper.NewCanBusMapper(c2, cmc)
	case config.SignalKType:
		amc := config.NewExpressionMappingConfig(cfgFile)
		m, err = mapper.NewAggregateMapper(c, amc)

	default:
		logger.GetLogger().Fatal(
			"Not a supported protocol",
			zap.String("Protocol", c.Protocol),
		)
		return
	}
	if err != nil {
		logger.GetLogger().Fatal(
			"Error while creating the mapper",
			zap.String("Config file", cfgFile),
			zap.String("Error", err.Error()),
		)
	}
	m.Map(subscriber, publisher)
}
