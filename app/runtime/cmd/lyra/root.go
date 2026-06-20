package main

import "github.com/spf13/cobra"

// RootCmd builds the cobra root command and wires every
// subcommand. Returns a fresh tree per call so tests can build
// isolated trees with no shared state between cases.
func (a *App) RootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "lyra",
		Short:         "Lyra — general-purpose agent runtime",
		Long:          "Lyra is a CLI / runtime for talking to LLMs with a curated set of coding tools, planning, persistent sessions, and a LYRA.md memory cascade.",
		SilenceUsage:  true, // each subcommand prints its own usage on user error
		SilenceErrors: true, // we render errors via App.fatalErr so cobra doesn't double-print
	}
	root.AddCommand(
		a.AgentsCmd(),
		a.ChatCmd(),
		a.HooksCmd(),
		a.ReplCmd(),
		a.MemoryCmd(),
		a.ServeCmd(),
		a.SessionCmd(),
		a.VersionCmd(),
	)
	return root
}
