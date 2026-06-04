package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ReplCmd is `lyra repl [--session ID]` — interactive multi-turn
// chat. The user types messages one line at a time; each line
// drives one turn, events stream to stdout/stderr as they arrive.
// Slash-prefixed input invokes REPL commands rather than the
// model:
//
//	/exit     exit the REPL (also: Ctrl+D / EOF)
//	/help     show available commands
//	/new      start a fresh session in this REPL
//	/plan ... run one plan-mode turn
//	/session  print the current session id
func (a *App) ReplCmd() *cobra.Command {
	var sessionID string
	cmd := &cobra.Command{
		Use:   "repl",
		Short: "Interactive multi-turn conversation.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensureRuntime(cmd.Context()); err != nil {
				return a.fatalErr(err)
			}
			r, err := NewReplRunner(a, sessionID)
			if err != nil {
				return a.fatalErr(err)
			}
			return r.Run(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "resume an existing session id (default: create a fresh one)")
	return cmd
}

// ReplRunner is the interactive loop's controller — owns the
// session it's bound to plus the App's IO + services.
type ReplRunner struct {
	app       *App
	sessionID string
}

// NewReplRunner resolves the session (resume given id or create
// fresh) and returns a ready runner. Errors propagate when
// resuming a non-existent session.
func NewReplRunner(app *App, requestedSession string) (*ReplRunner, error) {
	ctx := context.Background()
	var sessID string
	if requestedSession != "" {
		sess, err := app.rt.Session().Get(ctx, requestedSession)
		if err != nil {
			return nil, fmt.Errorf("resume session %q: %w", requestedSession, err)
		}
		sessID = sess.ID
	} else {
		cwd, _ := os.Getwd()
		sess, err := app.rt.Session().Create(ctx, "", cwd)
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
		sessID = sess.ID
	}
	return &ReplRunner{app: app, sessionID: sessID}, nil
}

// Run drives the REPL until /exit, Ctrl+D, or scanner error.
// Each line is either a slash command (handled inline) or a
// regular message (dispatched through [TurnRunner]).
func (r *ReplRunner) Run(ctx context.Context) error {
	fmt.Fprintf(r.app.Err, "[lyra] repl ready — session %s\n", r.sessionID)
	fmt.Fprintln(r.app.Err, "[lyra] type your message, or /help for commands.")

	scanner := bufio.NewScanner(r.app.In)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for {
		fmt.Fprintf(r.app.Err, "\n[%s] > ", shortSessionID(r.sessionID))
		if !scanner.Scan() {
			fmt.Fprintln(r.app.Err) // newline after EOF for clean shell prompt
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			if done := r.handleSlash(ctx, line); done {
				return nil
			}
			continue
		}

		NewTurnRunner(r.app, turnOptions{}).Run(ctx, r.sessionID, line)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return r.app.fatalErr(err)
	}
	return nil
}

// handleSlash dispatches one slash command. Returns true when the
// REPL should exit (/exit, /quit). /plan <msg> spawns a plan-mode
// TurnRunner since it consumes the rest of the line as the message.
func (r *ReplRunner) handleSlash(ctx context.Context, line string) (done bool) {
	if strings.HasPrefix(line, "/plan ") {
		msg := strings.TrimSpace(strings.TrimPrefix(line, "/plan "))
		if msg == "" {
			fmt.Fprintln(r.app.Err, "[lyra] usage: /plan <message>")
			return false
		}
		NewTurnRunner(r.app, turnOptions{PlanMode: true}).Run(ctx, r.sessionID, msg)
		return false
	}

	switch line {
	case "/exit", "/quit":
		return true
	case "/help":
		fmt.Fprintln(r.app.Err, "[lyra] commands: /exit  /help  /new  /plan <msg>  /session")
	case "/new":
		cwd, _ := os.Getwd()
		sess, err := r.app.rt.Session().Create(ctx, "", cwd)
		if err != nil {
			fmt.Fprintf(r.app.Err, "[lyra] create session: %s\n", err)
			return false
		}
		r.sessionID = sess.ID
		fmt.Fprintf(r.app.Err, "[lyra] switched to fresh session %s\n", sess.ID)
	case "/session":
		fmt.Fprintf(r.app.Err, "[lyra] current session: %s\n", r.sessionID)
	default:
		fmt.Fprintf(r.app.Err, "[lyra] unknown command %q (try /help)\n", line)
	}
	return false
}

// shortSessionID returns the first 8 characters of a session id
// for use in user-facing prompts. UUIDs are 36 chars; the first
// segment is enough to disambiguate visually without filling the
// prompt line.
func shortSessionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
