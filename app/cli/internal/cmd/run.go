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
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Tangerg/lynx/app/cli/internal/attachment"
	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/identity"
	"github.com/Tangerg/lynx/app/cli/internal/render"
	"github.com/Tangerg/lynx/app/cli/internal/resilience"
)

// renderer is what `run` needs to write a run out. Declared here, where it is
// consumed, so the render package owes the command nothing beyond these two
// methods.
type renderer interface {
	Render(client.Envelope) error
	Close() error
}

func newRunCommand(resolve backend, v *viper.Viper) *cobra.Command {
	var (
		asJSON     bool
		approveAll bool
		sessionID  string
		files      []string
	)
	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Run one prompt to completion and exit",
		Long: "run drives a single prompt without an interactive surface: it starts a run,\n" +
			"writes what happens as it happens, and exits when the run ends.\n\n" +
			"Anything piped in is appended to the prompt. --file attaches a local file as\n" +
			"typed context and can be repeated; attachment-only turns are valid.\n\n" +
			"A run that needs approval stops and says so. --approve-all answers yes to every\n" +
			"request instead, which is the only way an unattended run gets past one.",
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readSettings(v)
			if err != nil {
				return err
			}
			ws, err := workspace(cmd)
			if err != nil {
				return err
			}
			prompt, promptErr := prompt(cmd, args)
			if promptErr != nil && (!errors.Is(promptErr, errNoPrompt) || len(files) == 0) {
				return promptErr
			}
			attached, err := resolveAttachments(cmd.Context(), ws, files)
			if err != nil {
				return err
			}
			rt, err := resolve.open(cmd)
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
				Message:   client.Message{Text: prompt, Attachments: attached},
				Options:   value.RunOptions(),
			}, approveAll, value.UI.ReconnectAttempts)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Write newline-delimited JSON instead of text")
	cmd.Flags().BoolVar(&approveAll, "approve-all", false, "Approve every request the run makes")
	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "Run inside an existing session instead of a new one")
	cmd.Flags().StringArrayVarP(&files, "file", "f", nil, "Attach a local file (repeatable)")
	_ = cmd.RegisterFlagCompletionFunc("file", func(cmd *cobra.Command, _ []string, value string) ([]string, cobra.ShellCompDirective) {
		ws, err := workspace(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		resolver, err := attachment.New(ws)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		matches, err := resolver.Complete(cmd.Context(), value, attachment.DefaultLimit)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		out := make([]string, 0, len(matches))
		for _, match := range matches {
			out = append(out, match.Path+"\t"+match.Detail)
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func resolveAttachments(ctx context.Context, workspace string, paths []string) ([]client.Attachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	resolver, err := attachment.New(workspace)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]client.Attachment, 0, len(paths))
	for _, path := range paths {
		item, err := resolver.Resolve(ctx, path)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[item.Path]; duplicate {
			continue
		}
		if len(out) >= client.MaxMessageAttachments {
			return nil, fmt.Errorf("at most %d unique attachments are allowed", client.MaxMessageAttachments)
		}
		seen[item.Path] = struct{}{}
		out = append(out, item)
	}
	return out, nil
}

// follow drives a run to its end, answering each park and continuing on the
// stream the answer opens. A park is not an ending: the run id is stable across
// it, and only the stream is new.
func follow(ctx context.Context, rt client.Runtime, out renderer, start client.StartRun, approveAll bool, reconnectAttempts int) error {
	if start.RequestID == "" {
		requestID, err := identity.New("req")
		if err != nil {
			return err
		}
		start.RequestID = requestID
	}
	policy := resilience.Standard(reconnectAttempts)
	run, err := retryControlValue(ctx, policy, func() (client.Run, error) {
		return rt.StartRun(ctx, start)
	})
	if err != nil {
		return err
	}
	var runID atomic.Pointer[string]
	runID.Store(&run.ID)
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-finished:
			return
		case <-ctx.Done():
			if id := runID.Load(); id != nil {
				// Detached from the signal but bounded, so the cancellation request
				// can finish without leaving a goroutine behind indefinitely.
				cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				_ = rt.CancelRun(cancelCtx, *id)
			}
		}
	}()

	sequence := client.NewEventSequence(run.StartedAfter)
	failures := 0
	for {
		before := sequence.Cursor()
		stream, err := rt.FollowRun(ctx, client.FollowRun{RunID: run.ID, After: before})
		if err != nil {
			if retryErr := waitToReconnect(ctx, policy, &failures, false, err); retryErr != nil {
				return retryErr
			}
			continue
		}
		if stream == nil {
			return errors.New("runtime returned a nil event stream")
		}
		var interrupted client.Interaction
		finished := false
		var subscriptionErr error
		for envelope, streamErr := range stream {
			if streamErr != nil {
				subscriptionErr = streamErr
				break
			}
			result, err := sequence.Accept(envelope, func() error { return out.Render(envelope) })
			if err != nil {
				subscriptionErr = fmt.Errorf("accept runtime event at cursor %d: %w", envelope.Cursor, err)
				break
			}
			if !result.Applied {
				continue
			}
			switch event := envelope.Event.(type) {
			case client.RunInterrupted:
				interrupted = event.Interaction
			case client.RunFinished:
				finished = true
			}
		}
		progressed := sequence.Cursor() > before
		if finished {
			return out.Close()
		}
		if interrupted != nil {
			answer, interruptID, err := unattendedAnswer(interrupted, approveAll, start.SessionID)
			if err != nil {
				return err
			}
			resume := client.ResumeRun{RunID: run.ID, InterruptID: interruptID, Answer: answer}
			if err := retryControl(ctx, policy, func() error { return rt.ResumeRun(ctx, resume) }); err != nil {
				return err
			}
			failures = 0
			continue
		}
		if subscriptionErr == nil {
			subscriptionErr = fmt.Errorf("%w: runtime subscription ended without interrupting or finishing the run", client.ErrDisconnected)
		}
		if retryErr := waitToReconnect(ctx, policy, &failures, progressed, subscriptionErr); retryErr != nil {
			return retryErr
		}
	}
}

func retryControl(ctx context.Context, policy resilience.Reconnect, operation func() error) error {
	_, err := retryControlValue(ctx, policy, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

func retryControlValue[T any](ctx context.Context, policy resilience.Reconnect, operation func() (T, error)) (T, error) {
	var zero T
	for failure := 1; ; failure++ {
		value, err := operation()
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, client.ErrDisconnected) {
			return zero, err
		}
		delay, retry := policy.Next(failure, err)
		if !retry {
			return zero, err
		}
		if err := resilience.Wait(ctx, delay); err != nil {
			return zero, err
		}
	}
}

func waitToReconnect(ctx context.Context, policy resilience.Reconnect, failures *int, progressed bool, cause error) error {
	if progressed {
		*failures = 0
	}
	*failures++
	delay, ok := policy.Next(*failures, cause)
	if !ok {
		return cause
	}
	if err := resilience.Wait(ctx, delay); err != nil {
		return err
	}
	return nil
}

// decide answers a park. Unattended, a request is refused rather than guessed
// at, and the refusal says what would have allowed it.
func decide(approveAll bool) client.ApprovalAnswer {
	if approveAll {
		return client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberNone}
	}
	return client.ApprovalAnswer{Decision: client.ApprovalDeny, Reason: "declined: this run is unattended (rerun with --approve-all to allow it)"}
}

func unattendedAnswer(interaction client.Interaction, approveAll bool, sessionID string) (client.Answer, string, error) {
	switch item := interaction.(type) {
	case client.Approval:
		return decide(approveAll), item.InterruptID, nil
	case client.Question:
		return nil, item.InterruptID, fmt.Errorf("run needs answers to %q; continue it interactively with --session %s", item.Title, sessionID)
	default:
		return nil, "", errors.New("runtime returned an unknown interaction")
	}
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
