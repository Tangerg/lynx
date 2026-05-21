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
func cmdChat(args []string) int {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.SetOutput(stderr())
	fs.Usage = func() {
		fmt.Fprintln(stderr(), "Usage: lyra chat <message...>")
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

	return runTurn(rt, "cli-"+uuid.NewString(), message)
}

// runTurn dispatches one turn and prints the streamed events to
// stdout / stderr. Shared between `chat` (one-shot) and `repl`
// (loop). Returns 0 on TurnEnd success, non-zero on Error or turn
// failure.
func runTurn(rt *runtime, sessionID, message string) int {
	ctx := context.Background()

	handle, err := rt.chat.StartTurn(ctx, chat.StartTurnRequest{
		SessionID: sessionID,
		Message:   message,
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
