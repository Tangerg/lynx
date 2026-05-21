package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

// cmdChat is `lyra chat <message>` — one-shot, no session
// persistence beyond the in-memory chat-memory keyed by the
// auto-generated session id. Convenient for scripted use and for
// quick smoke tests.
//
// With --plan the runtime first asks the LLM for a step-by-step
// plan, prints it, and waits for the user's y/N before executing
// any tools. --auto-approve skips the prompt for unattended runs.
// --verbose disables the tool-output truncation that hides
// multi-line stdout / stderr behind a "+N more" footer.
func cmdChat(args []string) int {
	fs := newSubFlagSet("chat")
	planMode := fs.Bool("plan", false, "ask the LLM for a plan and prompt to approve before executing")
	autoApprove := fs.Bool("auto-approve", false, "with --plan: approve the plan without prompting")
	verbose := fs.Bool("verbose", false, "print full tool output (default: truncate after a few lines)")
	fs.Usage = func() {
		fmt.Fprintln(stderr(), "Usage: lyra chat [--plan [--auto-approve]] [--verbose] <message...>")
		fmt.Fprintln(stderr(), "Send a single user message and stream the assistant's reply to stdout.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	message := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if message == "" {
		fs.Usage()
		return 2
	}

	rt, err := newRuntime()
	if err != nil {
		return printErr(err)
	}

	return runTurnWithOptions(rt, "cli-"+uuid.NewString(), message, turnOptions{
		PlanMode:    *planMode,
		AutoApprove: *autoApprove,
		Verbose:     *verbose,
	})
}

// turnOptions configures one-off runTurn calls. Threading them
// through avoids growing runTurn's positional signature each time
// a new flag arrives.
type turnOptions struct {
	PlanMode    bool
	AutoApprove bool
	Verbose     bool
}

// runTurn dispatches one turn (no plan mode) — convenience wrapper
// used by the REPL when it doesn't need plan-related knobs.
func runTurn(rt *runtime, sessionID, message string) int {
	return runTurnWithOptions(rt, sessionID, message, turnOptions{})
}

// runTurnWithOptions dispatches one turn and prints the streamed
// events to stdout / stderr. Shared between `chat` (one-shot) and
// `repl` (loop).
//
// Plan-mode turns pause after [chat.PlanGenerated] — the function
// prompts the user (or auto-approves) and calls
// [chat.Service.ContinuePlan] before draining the rest of the
// stream.
//
// SIGINT during execution cancels the in-flight turn via
// [chat.Service.Cancel] rather than killing the process; the
// function returns once the runtime emits the resulting
// TurnEndCancelled. Subsequent signals during the same turn are
// ignored — the cancel is already in flight.
//
// Returns 0 on TurnEnd success, non-zero on Error / turn failure /
// user rejection.
func runTurnWithOptions(rt *runtime, sessionID, message string, opts turnOptions) int {
	ctx := context.Background()

	handle, err := rt.chat.StartTurn(ctx, chat.StartTurnRequest{
		SessionID: sessionID,
		Message:   message,
		PlanMode:  opts.PlanMode,
	})
	if err != nil {
		return printErr(err)
	}

	events, err := rt.chat.Events(ctx, handle)
	if err != nil {
		return printErr(err)
	}

	// Register SIGINT only for the duration of this turn. The signal
	// channel is nil-ed out after the first cancel so a stuck turn
	// can still be force-killed by a second Ctrl+C escalating to
	// the default os.Interrupt behaviour.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	exit := 0
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return exit
			}
			if newExit := handleEvent(rt, ctx, handle, ev, opts); newExit != 0 {
				exit = newExit
			}
		case <-sigCh:
			fmt.Fprintln(stderr(), "\n[lyra] cancelling...")
			_ = rt.chat.Cancel(ctx, handle)
			sigCh = nil // ignore further signals until TurnEnd
		}
	}
}

// handleEvent prints one event and returns the exit code adjustment
// (0 = leave alone, non-zero = update). Pulled out of the select
// loop so the loop body stays a clean two-case dispatch.
func handleEvent(rt *runtime, ctx context.Context, handle chat.TurnHandle, ev chat.Event, opts turnOptions) int {
	switch e := ev.(type) {
	case chat.TurnStart:
		fmt.Fprintf(stderr(), "[lyra] turn %s started (model=%s)\n", e.TurnID[:8], e.Model)
	case chat.PlanGenerated:
		fmt.Fprintln(stdout(), "\n---- proposed plan ----")
		fmt.Fprintln(stdout(), e.Plan)
		fmt.Fprintln(stdout(), "-----------------------")
		decision := decidePlan(opts.AutoApprove)
		if err := rt.chat.ContinuePlan(ctx, handle, decision); err != nil {
			fmt.Fprintf(stderr(), "[lyra] continue plan: %s\n", err)
		}
		if decision == chat.PlanReject {
			fmt.Fprintln(stderr(), "[lyra] plan rejected")
		}
	case chat.MessageDelta:
		fmt.Fprint(stdout(), e.Text)
	case chat.ToolCallStart:
		fmt.Fprintf(stderr(), "\n[lyra] tool start: %s\n", e.ToolName)
	case chat.ToolCallEnd:
		printToolOutput(e, opts.Verbose)
	case chat.ErrorEvent:
		fmt.Fprintf(stderr(), "\n[lyra] error: %s (%s)\n", e.Message, e.Code)
		return 1
	case chat.TurnEnd:
		fmt.Fprintf(stderr(), "\n[lyra] turn ended: %s (%s)\n", e.Reason, e.Duration)
		if e.Reason == chat.TurnEndErrored {
			return 1
		}
	}
	return 0
}

// printToolOutput renders the tool's stdout/stderr blob. In the
// default (terse) mode anything beyond [toolOutputMaxLines] is
// suppressed and replaced with a "+N more" footer — keeps a noisy
// `grep -r` from drowning the conversation. --verbose prints the
// raw blob.
func printToolOutput(e chat.ToolCallEnd, verbose bool) {
	if e.Err != "" {
		fmt.Fprintf(stderr(), "[lyra] tool end: error: %s\n", e.Err)
		return
	}
	if e.Output == "" {
		fmt.Fprintln(stderr(), "[lyra] tool end: (no output)")
		return
	}
	if verbose {
		fmt.Fprintf(stderr(), "[lyra] tool end:\n%s\n", strings.TrimRight(e.Output, "\n"))
		return
	}
	fmt.Fprintf(stderr(), "[lyra] tool end:\n%s\n", truncateOutput(e.Output, toolOutputMaxLines))
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

// decidePlan prompts the user for y/N (default: reject for safety).
// AutoApprove short-circuits the prompt — convenient for scripted
// pipelines that piped --plan output through a script.
func decidePlan(autoApprove bool) chat.PlanDecision {
	if autoApprove {
		fmt.Fprintln(stderr(), "[lyra] auto-approving plan")
		return chat.PlanApprove
	}
	fmt.Fprint(stdout(), "Proceed? [y/N] ")
	var answer string
	_, _ = fmt.Fscanln(stdin(), &answer)
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return chat.PlanApprove
	}
	return chat.PlanReject
}
