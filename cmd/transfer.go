package cmd

import (
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/sdnotify"
	"github.com/munnik/gosk/transfer"
	"github.com/spf13/cobra"
)

var (
	transferCmd = &cobra.Command{
		Use:   "transfer",
		Short: "transfer missing data",
		Long:  `This can transfer missing data from clients to a central server`,
	}
	transferRequestCmd = &cobra.Command{
		Use:   "request",
		Short: "request clients for data count and missing data",
		Long:  `request clients for data count in a certain period and to send missing data to the central server if the data count on the central server is less`,
		Run:   doTransferRequest,
	}
	transferRespondCmd = &cobra.Command{
		Use:   "respond",
		Short: "respond to data count and missing data requests",
		Long:  `respond to data count and missing data requests`,
		Run:   doTransferRespond,
	}
	transferOutboxCmd = &cobra.Command{
		Use:   "outbox",
		Short: "send and receive data through the outbox",
		Long:  `Deliver data by working through a list of what has not been acknowledged yet, rather than by comparing counts per period. Runs alongside request/respond, it does not replace them.`,
	}
	transferOutboxShipCmd = &cobra.Command{
		Use:   "ship",
		Short: "send unacknowledged data to the central server",
		Long:  `On a vessel: work through transfer_outbox oldest first, send each batch, and clear entries once the far end confirms it has stored them.`,
		Run:   doTransferOutboxShip,
	}
	transferOutboxReceiveCmd = &cobra.Command{
		Use:   "receive",
		Short: "store data shipped by vessels and acknowledge it",
		Long:  `On the central server: store what a vessel ships, and acknowledge it only once the write has committed.`,
		Run:   doTransferOutboxReceive,
	}
)

func init() {
	rootCmd.AddCommand(transferCmd)

	transferCmd.AddCommand(transferRequestCmd)
	transferCmd.AddCommand(transferRespondCmd)
	transferRespondCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	transferRespondCmd.MarkFlagRequired("publishURL")

	transferCmd.AddCommand(transferOutboxCmd)
	transferOutboxCmd.AddCommand(transferOutboxShipCmd)
	transferOutboxCmd.AddCommand(transferOutboxReceiveCmd)
}

func doTransferOutboxShip(cmd *cobra.Command, args []string) {
	c := config.NewTransferConfig(cfgFile)
	s := transfer.NewOutboxShipper(c)
	// Like the other transfer processes: it neither subscribes to nor
	// publishes nanomsg data, so there is no first message to hang
	// readiness off. Having read its configuration and connected is the
	// meaningful ready state.
	sdnotify.Ready()
	s.Run()
}

func doTransferOutboxReceive(cmd *cobra.Command, args []string) {
	c := config.NewTransferConfig(cfgFile)
	r := transfer.NewOutboxReceiver(c)
	sdnotify.Ready()
	r.Run()
}

func doTransferRequest(cmd *cobra.Command, args []string) {
	c := config.NewTransferConfig(cfgFile)
	w := transfer.NewTransferRequester(c)
	// Unlike every other processor type, this one neither subscribes to
	// nor publishes any nanomsg data - Run polls the database/its peers
	// directly - so there's no "first message" to hang readiness off of.
	// It's ready as soon as it's constructed.
	sdnotify.Ready()
	w.Run()
}

func doTransferRespond(cmd *cobra.Command, args []string) {
	c := config.NewTransferConfig(cfgFile)
	w := transfer.NewTransferResponder(c)
	// Like doTransferRequest above, but more so: this one only ever
	// publishes in response to a request from a peer, so gating readiness
	// on nanomsg.Publisher's first-successful-send (see pub.go's send)
	// would mean a peer that simply never asks anything - not unusual, this
	// is the responder side - leaves the unit stuck below TimeoutStartSec
	// forever. Listening for requests is itself the meaningful ready state
	// here.
	sdnotify.Ready()
	w.Run(nanomsg.NewPublisher[message.Mapped](publishURL))
}
