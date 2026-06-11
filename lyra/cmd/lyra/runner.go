package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/engine/chat"
)

// turnOptions is the per-turn knobs both `chat` and `repl` pass to
// [TurnRunner.Run]. Plan-mode + auto-approve drive the plan-mode
// approval prompt; Verbose disables tool-output truncation.
type turnOptions struct {
	PlanMode    bool
	AutoApprove bool
	Verbose     bool

	// MaxBudget / MaxCostUSD cap the turn by tokens / dollars (0 = no
	// cap). On overrun the turn stops cleanly and ends budget_exceeded.
	MaxBudget  int64
	MaxCostUSD float64
}

// TurnRunner drives a single chat turn end-to-end: starts it,
// drains the event channel, prints events to the App's IO
// streams, and handles SIGINT by canceling the in-flight turn
// (rather than the whole process).
//
// One instance per turn — TurnRunner holds the active TurnHandle
// so it doesn't have to thread it through every helper.
type TurnRunner struct {
	app  *App
	opts turnOptions

	// exit tracks the cumulative exit code over the event stream;
	// 1 on errors, 0 otherwise.
	exit int
}

// NewTurnRunner wires the runner to an App and per-turn options.
func NewTurnRunner(app *App, opts turnOptions) *TurnRunner {
	return &TurnRunner{app: app, opts: opts}
}

// Run starts a turn for sessionID + message, drains its events,
// and returns the resulting exit code (0 ok / 1 errored).
//
// Cancellation: SIGINT during the turn calls
// [chat.Service.Cancel](handle) so the runtime emits a clean
// TurnEnd(Canceled). A second SIGINT escalates to the default
// kill — for wedged turns.
func (r *TurnRunner) Run(ctx context.Context, sessionID, message string) int {
	// Resolve the session's cwd so fs/bash tools run in the session's
	// project directory rather than the engine default — same contract
	// as the runs.start wire path.
	sess, err := r.app.rt.Session().Get(ctx, sessionID)
	if err != nil {
		fmt.Fprintf(r.app.Err, "lyra: %s\n", err)
		return 1
	}
	handle, err := r.app.rt.Chat().StartTurn(ctx, chat.StartTurnRequest{
		SessionID:  sessionID,
		Message:    message,
		Cwd:        sess.Cwd,
		PlanMode:   r.opts.PlanMode,
		MaxBudget:  r.opts.MaxBudget,
		MaxCostUSD: r.opts.MaxCostUSD,
	})
	if err != nil {
		fmt.Fprintf(r.app.Err, "lyra: %s\n", err)
		return 1
	}
	events, err := r.app.rt.Chat().Events(ctx, handle)
	if err != nil {
		fmt.Fprintf(r.app.Err, "lyra: %s\n", err)
		return 1
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	// Handle SIGINT off the event loop: the first one cancels the turn
	// (the runtime then emits TurnEnd(Canceled) and closes the stream,
	// ending the range below); a second escalates to the default kill.
	// sigCtx ties this goroutine's lifetime to the turn so it exits when
	// the stream drains.
	sigCtx, stopSig := context.WithCancel(ctx)
	defer stopSig()
	go func() {
		select {
		case <-sigCh:
			fmt.Fprintln(r.app.Err, "\n[lyra] canceling...")
			_ = r.app.rt.Chat().Cancel(ctx, handle)
			signal.Stop(sigCh)
		case <-sigCtx.Done():
		}
	}()

	for ev := range events {
		r.dispatch(ctx, handle, ev)
	}
	return r.exit
}

// dispatch routes one event to the right per-type renderer. Each
// renderer is a method on *TurnRunner so it has access to the
// App's streams + the active TurnHandle.
func (r *TurnRunner) dispatch(ctx context.Context, handle chat.TurnHandle, ev chat.Event) {
	switch e := ev.(type) {
	case chat.TurnStart:
		fmt.Fprintf(r.app.Err, "[lyra] turn %s started (model=%s)\n", e.TurnID[:8], e.Model)
	case chat.PlanGenerated:
		r.renderPlan(e)
	case chat.TurnInterrupted:
		r.handleInterrupt(ctx, handle, e)
	case chat.MessageDelta:
		fmt.Fprint(r.app.Out, e.Text)
	case chat.ReasoningDelta:
		// Render extended thinking only when --verbose is on so the
		// terse-mode UX stays focused on the model's final answer.
		// Marked with a `[think]` prefix per line so it doesn't mix
		// with the assistant text on stdout.
		if r.opts.Verbose {
			fmt.Fprintf(r.app.Err, "[think] %s", e.Text)
		}
	case chat.ToolCallStart:
		fmt.Fprintf(r.app.Err, "\n[lyra] tool start: %s\n", e.ToolName)
	case chat.ToolCallEnd:
		r.renderToolEnd(e)
	case chat.ErrorEvent:
		fmt.Fprintf(r.app.Err, "\n[lyra] error: %s (%s)\n", e.Message, e.Code)
		r.exit = 1
	case chat.TurnEnd:
		r.renderTurnEnd(e)
		if e.Reason == chat.TurnEndErrored {
			r.exit = 1
		}
	}
}

// renderPlan prints the proposed plan. The approval prompt itself fires
// later, on the [chat.TurnInterrupted] that follows once the turn has
// actually parked (R model) — see [TurnRunner.handleInterrupt].
func (r *TurnRunner) renderPlan(e chat.PlanGenerated) {
	fmt.Fprintln(r.app.Out, "\n---- proposed plan ----")
	fmt.Fprintln(r.app.Out, e.Plan)
	fmt.Fprintln(r.app.Out, "-----------------------")
}

// handleInterrupt answers a parked turn (HITL R model): it describes the
// pending request — a gated tool call (prints tool + args) or a plan
// (already printed by renderPlan) — prompts for approval, and forwards
// the decision via [chat.Service.Resume]. The continuation streams onto
// the same event channel the caller is still draining.
func (r *TurnRunner) handleInterrupt(ctx context.Context, handle chat.TurnHandle, e chat.TurnInterrupted) {
	for _, in := range e.Interrupts {
		if in.Kind == "approval" {
			if p, ok := in.Payload.(chat.ApprovalPrompt); ok {
				fmt.Fprintf(r.app.Out, "\n---- approve tool: %s ----\n%s\n-----------------------\n", p.ToolName, p.Arguments)
			}
		}
	}
	approved := r.decide()
	if err := r.app.rt.Chat().Resume(ctx, handle, engine.InterruptResolution{Approved: approved}); err != nil {
		fmt.Fprintf(r.app.Err, "[lyra] resume: %s\n", err)
	}
	if !approved {
		fmt.Fprintln(r.app.Err, "[lyra] declined")
	}
}

// renderTurnEnd prints the per-turn summary line — reason + wall
// clock plus the token roll-up when the provider reported one.
// Zero usage is omitted (stub models / mocked endpoints) so the
// line stays compact when there's nothing to show.
func (r *TurnRunner) renderTurnEnd(e chat.TurnEnd) {
	u := e.TokenUsage
	if u.PromptTokens == 0 && u.CompletionTokens == 0 && u.ReasoningTokens == 0 {
		fmt.Fprintf(r.app.Err, "\n[lyra] turn ended: %s (%s)\n", e.Reason, e.Duration)
		return
	}
	fmt.Fprintf(r.app.Err, "\n[lyra] turn ended: %s (%s) — tokens: %d in / %d out", e.Reason, e.Duration, u.PromptTokens, u.CompletionTokens)
	if u.ReasoningTokens > 0 {
		fmt.Fprintf(r.app.Err, " (%d reasoning)", u.ReasoningTokens)
	}
	fmt.Fprintln(r.app.Err)
}

// renderToolEnd prints the tool's output, truncated to
// [toolOutputMaxLines] in terse mode and verbatim under --verbose.
func (r *TurnRunner) renderToolEnd(e chat.ToolCallEnd) {
	if e.Err != "" {
		fmt.Fprintf(r.app.Err, "[lyra] tool end: error: %s\n", e.Err)
		return
	}
	if e.Output == "" {
		fmt.Fprintln(r.app.Err, "[lyra] tool end: (no output)")
		return
	}
	if r.opts.Verbose {
		fmt.Fprintf(r.app.Err, "[lyra] tool end:\n%s\n", strings.TrimRight(e.Output, "\n"))
		return
	}
	fmt.Fprintf(r.app.Err, "[lyra] tool end:\n%s\n", truncateOutput(e.Output, toolOutputMaxLines))
}

// decide prompts the user for y/N (default: deny). With AutoApprove the
// prompt is skipped — convenient for unattended pipelines like
// `lyra chat --plan --auto-approve "..."`. Returns true on approval.
func (r *TurnRunner) decide() bool {
	if r.opts.AutoApprove {
		fmt.Fprintln(r.app.Err, "[lyra] auto-approving")
		return true
	}
	fmt.Fprint(r.app.Out, "Proceed? [y/N] ")
	answer := readLine(r.app.In)
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}

// readLine reads a single line of input, swallowing the trailing
// newline. Uses Fscanln when the reader supports word-scanning,
// falls back to a byte-at-a-time read otherwise. Tests pass
// strings.Reader; production passes os.Stdin.
func readLine(r io.Reader) string {
	var line string
	_, _ = fmt.Fscanln(r, &line)
	return line
}

// toolOutputMaxLines is the default per-tool-call line cap before
// terse mode collapses the rest. 8 lines is enough to show a
// small file's contents or a short error stack without dominating
// the screen.
const toolOutputMaxLines = 8

// truncateOutput returns at most maxLines lines from s, appending
// a "... +N more" marker when truncation happened. Trailing
// newlines on the input are trimmed first so the line count
// matches what the user perceives.
func truncateOutput(s string, maxLines int) string {
	trimmed := strings.TrimRight(s, "\n")
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= maxLines {
		return trimmed
	}
	head := strings.Join(lines[:maxLines], "\n")
	return fmt.Sprintf("%s\n... (+%d more lines, --verbose to see all)", head, len(lines)-maxLines)
}
