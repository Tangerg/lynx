package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/render"
)

// renderer is what `run` needs to write a run out. Declared here, where it is
// consumed, so the render package owes the command nothing beyond these two
// methods.
type renderer interface {
	Render(client.Event) error
	Close() error
}

func newRunCommand(resolve backend) *cobra.Command {
	var (
		asJSON     bool
		approveAll bool
		sessionID  string
	)
	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Run one prompt to completion and exit",
		Long: "run drives a single prompt without an interactive surface: it starts a run,\n" +
			"writes what happens as it happens, and exits when the run ends.\n\n" +
			"Anything piped in is appended to the prompt, so a file, a diff or a log can be\n" +
			"handed over as context.\n\n" +
			"A run that needs approval stops and says so. --approve-all answers yes to every\n" +
			"request instead, which is the only way an unattended run gets past one.",
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := resolve(cmd)
			if err != nil {
				return err
			}
			prompt, err := prompt(cmd, args)
			if err != nil {
				return err
			}
			ws, err := workspace(cmd)
			if err != nil {
				return err
			}
			session, err := sessionFor(cmd.Context(), rt, sessionID, ws)
			if err != nil {
				return err
			}

			var out renderer = render.NewText(cmd.OutOrStdout())
			if asJSON {
				out = render.NewJSON(cmd.OutOrStdout())
			}
			return follow(cmd.Context(), rt, out, client.StartRun{
				SessionID: session.ID,
				Prompt:    prompt,
			}, approveAll)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Write newline-delimited JSON instead of text")
	cmd.Flags().BoolVar(&approveAll, "approve-all", false, "Approve every request the run makes")
	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "Run inside an existing session instead of a new one")
	cmd.Flags().StringP("cwd", "C", "", "Workspace directory for a new session (default: current directory)")
	return cmd
}

// follow drives a run to its end, answering each park and continuing on the
// stream the answer opens. A park is not an ending: the run id is stable across
// it, and only the stream is new.
func follow(ctx context.Context, rt client.Runtime, out renderer, start client.StartRun, approveAll bool) error {
	stream, err := rt.StartRun(ctx, start)
	if err != nil {
		return err
	}

	// The run's id is only known once the stream opens, and the signal watcher
	// below reads it from another goroutine.
	var runID atomic.Pointer[string]
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	go func() {
		<-ctx.Done()
		if id := runID.Load(); id != nil {
			// Detached from ctx on purpose: cancelling a run is a request that
			// must outlive the signal that asked for it.
			_ = rt.CancelRun(context.WithoutCancel(ctx), *id)
		}
	}()

	for stream != nil {
		var parked *client.Approval
		for ev, err := range stream {
			if err != nil {
				return err
			}
			if started, ok := ev.(client.RunStarted); ok {
				id := started.RunID
				runID.Store(&id)
			}
			if err := out.Render(ev); err != nil {
				return err
			}
			if p, ok := ev.(client.RunParked); ok {
				parked = &p.Approval
				break
			}
		}
		stream = nil
		if parked == nil {
			break
		}

		id := runID.Load()
		if id == nil {
			return errors.New("run parked before it reported its id")
		}
		stream, err = rt.ResumeRun(ctx, client.ResumeRun{
			RunID:       *id,
			InterruptID: parked.InterruptID,
			Decision:    decide(approveAll),
		})
		if err != nil {
			return err
		}
	}
	return out.Close()
}

// decide answers a park. Unattended, a request is refused rather than guessed
// at, and the refusal says what would have allowed it.
func decide(approveAll bool) client.Decision {
	if approveAll {
		return client.Decision{Approved: true}
	}
	return client.Decision{Reason: "declined: this run is unattended (rerun with --approve-all to allow it)"}
}

// prompt assembles the prompt from arguments and anything piped in.
func prompt(cmd *cobra.Command, args []string) (string, error) {
	parts := make([]string, 0, 2)
	if given := strings.TrimSpace(strings.Join(args, " ")); given != "" {
		parts = append(parts, given)
	}
	piped, err := piped(cmd.InOrStdin())
	if err != nil {
		return "", err
	}
	if piped != "" {
		parts = append(parts, piped)
	}
	if len(parts) == 0 {
		return "", errNoPrompt
	}
	return strings.Join(parts, "\n\n"), nil
}

// piped reads stdin when it is not a terminal. A terminal is left alone: a
// prompt-less `lyra run` should say so, not sit there looking hung.
func piped(in io.Reader) (string, error) {
	if f, ok := in.(*os.File); ok {
		info, err := f.Stat()
		if err != nil {
			return "", fmt.Errorf("inspect stdin: %w", err)
		}
		if info.Mode()&os.ModeCharDevice != 0 {
			return "", nil
		}
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}
