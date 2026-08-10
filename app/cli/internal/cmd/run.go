package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Tangerg/lynx/app/cli/internal/attachment"
	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/render"
	"github.com/Tangerg/lynx/app/cli/internal/requestid"
)

// renderer is what `run` needs to write a run out. Declared here, where it is
// consumed, so the render package owes the command nothing beyond these two
// methods.
type renderer interface {
	Render(client.Envelope) error
	Close() error
}

func newRunCommand(provider runtimeProvider, v *viper.Viper) *cobra.Command {
	flags := new(runFlags)
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
			return flags.execute(cmd, args, provider, v)
		},
	}
	flags.register(cmd)
	_ = cmd.RegisterFlagCompletionFunc("file", completeRunFile)
	return cmd
}

type runFlags struct {
	asJSON     bool
	approveAll bool
	sessionID  string
	files      []string
}

func (f *runFlags) register(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.asJSON, "json", false, "Write newline-delimited JSON instead of text")
	cmd.Flags().BoolVar(&f.approveAll, "approve-all", false, "Approve every request the run makes")
	cmd.Flags().StringVarP(&f.sessionID, "session", "s", "", "Run inside an existing session instead of a new one")
	cmd.Flags().StringArrayVarP(&f.files, "file", "f", nil, "Attach a local file (repeatable)")
}

func (f *runFlags) execute(cmd *cobra.Command, args []string, provider runtimeProvider, v *viper.Viper) error {
	value, err := readSettings(v)
	if err != nil {
		return err
	}
	workspacePath, err := workspace(cmd)
	if err != nil {
		return err
	}
	message, err := f.message(cmd, args, workspacePath)
	if err != nil {
		return err
	}
	runtime, err := provider.Open(cmd)
	if err != nil {
		return err
	}
	session, err := sessionFor(cmd.Context(), runtime, f.sessionID, workspacePath)
	if err != nil {
		return err
	}
	return follow(cmd.Context(), runtime, runRenderer(cmd, f.asJSON), client.StartRun{
		SessionID: session.ID, Message: message, Options: value.RunOptions(),
	}, f.approveAll, value.UI.ReconnectAttempts)
}

func (f *runFlags) message(cmd *cobra.Command, args []string, workspace string) (client.Message, error) {
	text, textErr := prompt(cmd, args)
	if textErr != nil && (!errors.Is(textErr, errNoPrompt) || len(f.files) == 0) {
		return client.Message{}, textErr
	}
	attached, err := resolveAttachments(cmd.Context(), workspace, f.files)
	if err != nil {
		return client.Message{}, err
	}
	return client.Message{Text: text, Attachments: attached}, nil
}

func runRenderer(cmd *cobra.Command, asJSON bool) renderer {
	if asJSON {
		return render.NewJSON(cmd.OutOrStdout())
	}
	return render.NewText(cmd.OutOrStdout())
}

func completeRunFile(cmd *cobra.Command, _ []string, value string) ([]string, cobra.ShellCompDirective) {
	workspacePath, err := workspace(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	resolver, err := attachment.New(workspacePath)
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
	if err := ensureRequestID(&start); err != nil {
		return err
	}
	policy := reconnect.New(reconnectAttempts)
	guard := watchRunCancellation(ctx, rt, policy, client.CancelRun{SessionID: start.SessionID, RequestID: start.RequestID})
	cancelOnExit := true
	defer func() { guard.Close(cancelOnExit) }()

	run, err := reconnect.ControlValue(ctx, policy, func() (client.Run, error) {
		return rt.StartRun(ctx, start)
	})
	if err != nil {
		return err
	}
	finished, err := driveRun(ctx, rt, out, start, run, approveAll, policy)
	if finished {
		cancelOnExit = false
	}
	return err
}

func ensureRequestID(start *client.StartRun) error {
	if start.RequestID != "" {
		return nil
	}
	requestID, err := requestid.New()
	if err != nil {
		return err
	}
	start.RequestID = requestID
	return nil
}

type runCancelGuard struct {
	exit chan bool
	done chan struct{}
}

func watchRunCancellation(
	ctx context.Context,
	rt client.Runtime,
	policy reconnect.Policy,
	request client.CancelRun,
) *runCancelGuard {
	guard := &runCancelGuard{exit: make(chan bool, 1), done: make(chan struct{})}
	go func() {
		defer close(guard.done)
		shouldCancel := true
		select {
		case shouldCancel = <-guard.exit:
		case <-ctx.Done():
		}
		if !shouldCancel {
			return
		}
		cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = reconnect.Control(cancelCtx, policy, func() error {
			return rt.CancelRun(cancelCtx, request)
		})
	}()
	return guard
}

func (g *runCancelGuard) Close(cancel bool) {
	if g == nil {
		return
	}
	g.exit <- cancel
	<-g.done
}

func driveRun(
	ctx context.Context,
	rt client.Runtime,
	out renderer,
	start client.StartRun,
	run client.Run,
	approveAll bool,
	policy reconnect.Policy,
) (bool, error) {
	conversation := client.NewConversationAt(run.StartedAfter)
	failures := 0
	for {
		before := conversation.Cursor()
		stream, err := rt.FollowRun(ctx, client.FollowRun{RunID: run.ID, After: before})
		if err != nil {
			if retryErr := waitToReconnect(ctx, policy, &failures, false, err); retryErr != nil {
				return false, retryErr
			}
			continue
		}
		if stream == nil {
			return false, errors.New("runtime returned a nil event stream")
		}
		state := consumeSubscription(stream, conversation, out)
		progressed := conversation.Cursor() > before
		if state.finished {
			return true, out.Close()
		}
		if state.interrupted != nil {
			if err := resumeUnattended(ctx, rt, policy, run.ID, start.SessionID, state.interrupted, approveAll); err != nil {
				return false, err
			}
			failures = 0
			continue
		}
		if state.err == nil {
			state.err = fmt.Errorf("%w: runtime subscription ended without interrupting or finishing the run", client.ErrDisconnected)
		}
		if retryErr := waitToReconnect(ctx, policy, &failures, progressed, state.err); retryErr != nil {
			return false, retryErr
		}
	}
}

type subscriptionState struct {
	interrupted client.Interaction
	finished    bool
	err         error
}

func consumeSubscription(stream client.Stream, conversation *client.Conversation, out renderer) subscriptionState {
	var state subscriptionState
	for envelope, streamErr := range stream {
		if streamErr != nil {
			state.err = streamErr
			break
		}
		result, err := conversation.ApplyEnvelope(envelope)
		if err != nil {
			state.err = fmt.Errorf("accept runtime event at cursor %d: %w", envelope.Cursor, err)
			break
		}
		if !result.Applied {
			continue
		}
		if err := out.Render(envelope); err != nil {
			state.err = err
			break
		}
		switch event := envelope.Event.(type) {
		case client.RunInterrupted:
			state.interrupted = event.Interaction
		case client.RunFinished:
			state.finished = true
		default:
		}
	}
	return state
}

func resumeUnattended(
	ctx context.Context,
	rt client.Runtime,
	policy reconnect.Policy,
	runID string,
	sessionID string,
	interaction client.Interaction,
	approveAll bool,
) error {
	answer, interruptID, err := unattendedAnswer(interaction, approveAll, sessionID)
	if err != nil {
		return err
	}
	resume := client.ResumeRun{RunID: runID, InterruptID: interruptID, Answer: answer}
	return reconnect.Control(ctx, policy, func() error { return rt.ResumeRun(ctx, resume) })
}

func waitToReconnect(ctx context.Context, policy reconnect.Policy, failures *int, progressed bool, cause error) error {
	if progressed {
		*failures = 0
	}
	*failures++
	delay, ok := policy.Next(*failures, cause)
	if !ok {
		return cause
	}
	if err := reconnect.Wait(ctx, delay); err != nil {
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
