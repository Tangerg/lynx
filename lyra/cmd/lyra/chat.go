package main

import (
	"context"
	"flag"
	"fmt"
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
func cmdChat(args []string) int {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.SetOutput(stderr())
	planMode := fs.Bool("plan", false, "ask the LLM for a plan and prompt to approve before executing")
	autoApprove := fs.Bool("auto-approve", false, "with --plan: approve the plan without prompting")
	fs.Usage = func() {
		fmt.Fprintln(stderr(), "Usage: lyra chat [--plan [--auto-approve]] <message...>")
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
	})
}

// turnOptions configures one-off runTurn calls. Threading them
// through avoids growing runTurn's positional signature each time
// a new flag arrives.
type turnOptions struct {
	PlanMode    bool
	AutoApprove bool
}

// runTurn dispatches one turn (no plan mode) — convenience wrapper
// used by the REPL when it doesn't need plan-related knobs.
func runTurn(rt *runtime, sessionID, message string) int {
	return runTurnWithOptions(rt, sessionID, message, turnOptions{})
}

// runTurnWithOptions dispatches one turn and prints the streamed
// events to stdout / stderr. Shared between `chat` (one-shot) and
// `repl` (loop). Plan-mode turns pause after [chat.PlanGenerated]
// — the function then prompts the user (or auto-approves) and
// calls [chat.Service.ContinuePlan] before draining the rest of
// the stream.
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

	exit := 0
	for ev := range events {
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
			if e.Err != "" {
				fmt.Fprintf(stderr(), "[lyra] tool end: error: %s\n", e.Err)
			} else {
				fmt.Fprintf(stderr(), "[lyra] tool end (%d bytes)\n", len(e.Output))
			}
		case chat.ErrorEvent:
			fmt.Fprintf(stderr(), "\n[lyra] error: %s (%s)\n", e.Message, e.Code)
			exit = 1
		case chat.TurnEnd:
			fmt.Fprintf(stderr(), "\n[lyra] turn ended: %s (%s)\n", e.Reason, e.Duration)
			if e.Reason == chat.TurnEndErrored {
				exit = 1
			}
		}
	}

	return exit
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
