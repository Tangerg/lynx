package main

import (
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// ChatCmd is `lyra chat <message...>` — one-shot, no session
// persistence beyond the in-memory chat-memory keyed by the
// auto-generated session id. With --plan the LLM first drafts a
// plan and waits for y/N approval; --auto-approve skips the
// prompt; --verbose disables tool-output truncation.
func (a *App) ChatCmd() *cobra.Command {
	var (
		planMode    bool
		autoApprove bool
		verbose     bool
	)
	cmd := &cobra.Command{
		Use:   "chat [message...]",
		Short: "Send one message and print the streamed reply.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensureRuntime(); err != nil {
				return a.fatalErr(err)
			}
			message := strings.TrimSpace(strings.Join(args, " "))
			if message == "" {
				return cmd.Usage()
			}
			runner := NewTurnRunner(a, turnOptions{
				PlanMode:    planMode,
				AutoApprove: autoApprove,
				Verbose:     verbose,
			})
			if runner.Run(cmd.Context(), "cli-"+uuid.NewString(), message) != 0 {
				return errSilenced
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&planMode, "plan", false, "ask the LLM for a plan and prompt to approve before executing")
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "with --plan: approve the plan without prompting")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "print full tool output (default: truncate after a few lines)")
	return cmd
}
