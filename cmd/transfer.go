package cmd

import (
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/sdnotify"
	"github.com/munnik/gosk/transfer"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	seedFrom string
	seedTo   string

	transferCmd = &cobra.Command{
		Use:   "transfer",
		Short: "deliver data from vessels to the central server through an outbox",
		Long: `Deliver a vessel's mapped data to the central server and make sure nothing is missing there.

The vessel's mapped writer (write database mapped, with "outbox: true" in its configuration) records which minute of data every row it writes belongs to. "send" sends the rows of those minutes and clears them once the central server, running "receive", has stored them and said so. "seed" compares history on both ends and puts what the central server is missing into the outbox; "digest" answers it.`,
	}
	transferSendCmd = &cobra.Command{
		Use:   "send",
		Short: "send unacknowledged data to the central server",
		Long:  `On a vessel: work through the outbox, newest minute first and then oldest, send each minute's rows, and record what the central server acknowledges.`,
		Run:   doTransferSend,
	}
	transferReceiveCmd = &cobra.Command{
		Use:   "receive",
		Short: "store data sent by vessels and acknowledge it",
		Long:  `On the central server: store what a vessel sends, and acknowledge it only once the write has committed. With --publishURL, what is stored is also published for other processors.`,
		Run:   doTransferReceive,
	}
	transferSeedCmd = &cobra.Command{
		Use:   "seed",
		Short: "find history the central server is missing and put it in the outbox",
		Long: `On a vessel, once: compare this vessel's history with the central server's without either sending the other its data, and put only what the central server turns out to be missing into the outbox.

Comparison is by summary. Both ends group their rows into buckets of time - days, then hours, then minutes - and exchange a count and a checksum per bucket. Buckets that agree are settled in one line each however much they contain; only buckets that disagree are opened up, and at the bottom the minutes that differ go into the outbox. What this costs therefore follows how much is missing, not how much history there is.

Run "send" afterwards, or alongside, to deliver what this enqueues. This command writes nothing but outbox entries, sends no data, and can be interrupted and run again.`,
		Run: doTransferSeed,
	}
	transferDigestCmd = &cobra.Command{
		Use:   "digest",
		Short: "answer vessels seeding their outbox",
		Long:  `On the central server: answer the summaries a vessel sends while seeding, saying which parts of history do not match what is stored here. Reads only.`,
		Run:   doTransferDigest,
	}
)

func init() {
	rootCmd.AddCommand(transferCmd)
	transferCmd.AddCommand(transferSendCmd)
	transferCmd.AddCommand(transferReceiveCmd)
	transferCmd.AddCommand(transferSeedCmd)
	transferCmd.AddCommand(transferDigestCmd)

	// Accepted on every subcommand because the NixOS module hands a
	// --publishURL to every processor that is not a writer; only receive
	// does anything with it.
	for _, c := range []*cobra.Command{transferSendCmd, transferReceiveCmd, transferSeedCmd, transferDigestCmd} {
		c.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL to publish stored data on (receive only). It listens for connections.")
	}
	transferSeedCmd.Flags().StringVar(&seedFrom, "from", "", "Start of the history to compare, RFC 3339 or YYYY-MM-DD. Default: the oldest row this vessel holds.")
	transferSeedCmd.Flags().StringVar(&seedTo, "to", "", "End of the history to compare (exclusive), RFC 3339 or YYYY-MM-DD. Default: the start of the current minute.")
}

func doTransferSend(cmd *cobra.Command, args []string) {
	c := transferConfig(true)
	s := transfer.NewSender(c)
	// Like the other transfer processes it neither subscribes to nor
	// publishes nanomsg data, so there is no first message to hang
	// readiness off. Having read its configuration is the meaningful
	// ready state.
	sdnotify.Ready()
	s.Run()
}

func doTransferReceive(cmd *cobra.Command, args []string) {
	c := transferConfig(false)
	r := transfer.NewReceiver(c)
	var publisher *nanomsg.Publisher[message.Mapped]
	if publishURL != "" {
		publisher = nanomsg.NewPublisher[message.Mapped](publishURL)
	}
	sdnotify.Ready()
	r.Run(publisher)
}

func doTransferSeed(cmd *cobra.Command, args []string) {
	c := transferConfig(true)
	from, err := parseSeedTime(seedFrom)
	if err != nil {
		logger.GetLogger().Fatal("Could not parse --from", zap.Error(err))
	}
	to, err := parseSeedTime(seedTo)
	if err != nil {
		logger.GetLogger().Fatal("Could not parse --to", zap.Error(err))
	}
	s := transfer.NewSeeder(c)
	sdnotify.Ready()
	// Unlike the others this one finishes. Exiting non-zero when part of
	// history went uncompared matters: the run has to be repeatable, and
	// an operator has to be able to tell a complete pass from one that
	// gave up on some ranges.
	if err := s.Run(from, to); err != nil {
		logger.GetLogger().Fatal("Could not finish seeding the outbox", zap.Error(err))
	}
}

func doTransferDigest(cmd *cobra.Command, args []string) {
	c := transferConfig(false)
	r := transfer.NewDigestResponder(c)
	sdnotify.Ready()
	r.Run()
}

// transferConfig reads the configuration and refuses to start on one that
// cannot work, rather than running and retrying forever on it.
func transferConfig(needsOrigin bool) *config.TransferConfig {
	c := config.NewTransferConfig(cfgFile)
	if err := c.Validate(needsOrigin); err != nil {
		logger.GetLogger().Fatal("Invalid transfer configuration", zap.String("Config", cfgFile), zap.Error(err))
	}
	return c
}

// parseSeedTime reads a --from/--to value; empty means zero, which the
// seeder treats as its default.
func parseSeedTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}
