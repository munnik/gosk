package cmd

import (
	"fmt"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/transfer"
	"github.com/spf13/cobra"
)

var (
	transferCmd = &cobra.Command{
		Use:   "transfer",
		Short: "transfer missing data",
		Long:  `This can transfer missing data from clients to a central server`,
	}
	transferPublishCmd = &cobra.Command{
		Use:   "publish",
		Short: "ask clients for missing data",
		Long:  `asks clients to send missing data to the central server`,
		Run:   doTransferPublish,
	}
	transferSubscribeCmd = &cobra.Command{
		Use:   "subscribe",
		Short: "listen for missing data requests",
		Long:  `listens for missing data requests from the central server`,
		Run:   doTransferSubscribe,
	}
)

func init() {
	rootCmd.AddCommand(transferCmd)

	transferCmd.AddCommand(transferPublishCmd)
	transferCmd.AddCommand(transferSubscribeCmd)
	transferSubscribeCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	transferSubscribeCmd.MarkFlagRequired("publishURL")

}

func doTransferPublish(cmd *cobra.Command, args []string) {
	fmt.Println("test")

}
func doTransferSubscribe(cmd *cobra.Command, args []string) {
	c := config.NewTranferConfig(cfgFile)
	w := transfer.NewTransferSubscriber(c)
	w.ReadCommands(nanomsg.NewPub(publishURL))

}
