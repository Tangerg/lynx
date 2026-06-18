package main

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ChatCmd is `lyra chat <message...>` — one-shot: it creates a real
// session (so the turn's history and cwd persist like any other) and
// runs a single turn against it. --auto-approve approves gated tool
// calls without prompting; --verbose disables tool-output truncation.
func (a *App) ChatCmd() *cobra.Command {
	var (
		autoApprove bool
		verbose     bool
		maxBudget   int64
		maxCostUSD  float64
	)
	cmd := &cobra.Command{
		Use:   "chat [message...]",
		Short: "Send one message and print the streamed reply.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensureRuntime(cmd.Context()); err != nil {
				return a.fatalErr(err)
			}
			message := strings.TrimSpace(strings.Join(args, " "))
			if message == "" {
				return cmd.Usage()
			}
			// A real session (not a fabricated id): the ses_ id convention
			// holds and the chat-memory rows stay attached to a session the
			// session surface can list / delete.
			cwd, _ := os.Getwd()
			sess, err := a.rt.Session().Create(cmd.Context(), "", cwd)
			if err != nil {
				return a.fatalErr(err)
			}
			runner := NewTurnRunner(a, turnOptions{
				AutoApprove: autoApprove,
				Verbose:     verbose,
				MaxBudget:   maxBudget,
				MaxCostUSD:  maxCostUSD,
			})
			if runner.Run(cmd.Context(), sess.ID, message) != 0 {
				return errSilenced
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "approve gated tool calls without prompting")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "print full tool output (default: truncate after a few lines)")
	cmd.Flags().Int64Var(&maxBudget, "max-budget", 0, "stop the turn after this many tokens (0 = unlimited)")
	cmd.Flags().Float64Var(&maxCostUSD, "max-cost", 0, "stop the turn after this many USD (0 = unlimited; needs pricing)")
	return cmd
}
