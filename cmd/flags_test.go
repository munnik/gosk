package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func eachCommand(c *cobra.Command, f func(*cobra.Command)) {
	f(c)
	for _, sub := range c.Commands() {
		eachCommand(sub, f)
	}
}

// TestSubscribeURLFlagIsRepeatable pins the command line contract the
// fleet's nix configuration depends on. services.gosk.processors' own
// subscribeTo turns into one --subscribeURL argument per processor a unit
// subscribes to, so the flag has to keep exactly that name and has to
// collect every occurrence rather than keeping the last one. Getting
// either wrong does not fail any build: it fails at runtime, on every
// gosk unit on every vessel at once, with "unknown flag" - or worse,
// silently, by quietly reading only one of the publishers the unit was
// supposed to read.
func TestSubscribeURLFlagIsRepeatable(t *testing.T) {
	var checked int
	eachCommand(rootCmd, func(c *cobra.Command) {
		flag := c.Flags().Lookup("subscribeURL")
		if flag == nil {
			return
		}
		checked++

		if flag.Value.Type() != "stringSlice" {
			t.Errorf("%v: --subscribeURL is a %v, it must collect every occurrence", c.CommandPath(), flag.Value.Type())
			return
		}
		if flag.Shorthand != "s" {
			t.Errorf("%v: --subscribeURL lost its -s shorthand", c.CommandPath())
		}

		subscribeURLs = nil
		if err := c.Flags().Parse([]string{
			"--subscribeURL", "tcp://127.0.0.1:30001",
			"--subscribeURL", "tcp://127.0.0.1:30002",
			"--subscribeURL", "tcp://127.0.0.1:30003",
		}); err != nil {
			t.Errorf("%v: could not parse repeated --subscribeURL: %v", c.CommandPath(), err)
			return
		}
		if len(subscribeURLs) != 3 {
			t.Errorf("%v: parsed %d of 3 repeated --subscribeURL arguments: %v", c.CommandPath(), len(subscribeURLs), subscribeURLs)
		}
	})

	if checked == 0 {
		t.Fatal("no command has a --subscribeURL flag at all")
	}
}

// TestProxyCommandIsGone is the other half of that contract: a
// configuration that still asks for the removed proxy command has to fail
// loudly rather than be silently accepted.
func TestProxyCommandIsGone(t *testing.T) {
	eachCommand(rootCmd, func(c *cobra.Command) {
		if c.Name() == "proxy" {
			t.Errorf("the proxy command is back: %v", c.CommandPath())
		}
	})
}
