package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

// cmdRepl is `lyra repl` — interactive multi-turn chat against a
// single session. The user types messages one line at a time;
// each line drives one turn, events stream to stdout/stderr as
// they arrive. Slash-prefixed input invokes REPL commands rather
// than the model:
//
//	/exit     exit the REPL (also: Ctrl+D / EOF)
//	/help     show available commands
//	/new      start a fresh session in this REPL
//	/session  print the current session id
//
// The session is created (or resumed via --session) once at REPL
// start; every turn shares the same session id so chat-memory
// links them.
func cmdRepl(args []string) int {
	fs := newSubFlagSet("repl")
	sessionFlag := fs.String("session", "", "resume an existing session id (default: create a fresh one)")
	fs.Usage = func() {
		fmt.Fprintln(stderr(), "Usage: lyra repl [--session ID]")
		fmt.Fprintln(stderr(), "Start an interactive multi-turn conversation. Slash commands: /exit /help /new /session.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	rt, err := newRuntime()
	if err != nil {
		return printErr(err)
	}

	sessionID, err := resolveSession(rt, *sessionFlag)
	if err != nil {
		return printErr(err)
	}

	fmt.Fprintf(stderr(), "[lyra] repl ready — session %s\n", sessionID)
	fmt.Fprintln(stderr(), "[lyra] type your message, or /help for commands.")

	scanner := bufio.NewScanner(stdin())
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for {
		fmt.Fprint(stderr(), "\n> ")
		if !scanner.Scan() {
			// Ctrl+D / EOF
			fmt.Fprintln(stderr())
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			// /plan <msg> is the only slash command that runs a turn —
			// the others are pure REPL state changes. Handled inline
			// because the message body comes from the same input line.
			if strings.HasPrefix(line, "/plan ") {
				msg := strings.TrimSpace(strings.TrimPrefix(line, "/plan "))
				if msg == "" {
					fmt.Fprintln(stderr(), "[lyra] usage: /plan <message>")
					continue
				}
				runTurnWithOptions(rt, sessionID, msg, turnOptions{PlanMode: true})
				continue
			}
			done, newSession := handleSlashCommand(rt, line, sessionID)
			if done {
				return 0
			}
			if newSession != "" {
				sessionID = newSession
			}
			continue
		}

		runTurn(rt, sessionID, line)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return printErr(err)
	}
	return 0
}

// resolveSession either resumes the supplied id (asserts it exists)
// or creates a fresh anonymous session. Returns the session id the
// REPL should bind every turn to.
func resolveSession(rt *runtime, requested string) (string, error) {
	ctx := context.Background()
	if requested != "" {
		sess, err := rt.session.Get(ctx, requested)
		if err != nil {
			return "", fmt.Errorf("resume session %q: %w", requested, err)
		}
		return sess.ID, nil
	}
	sess, err := rt.session.Create(ctx, "")
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return sess.ID, nil
}

// handleSlashCommand interprets the REPL meta-commands. Returns
// (done, newSessionID). done=true exits the REPL; newSessionID
// non-empty rebinds the loop to a fresh session id.
func handleSlashCommand(rt *runtime, line, current string) (done bool, newSessionID string) {
	switch line {
	case "/exit", "/quit":
		return true, ""
	case "/help":
		fmt.Fprintln(stderr(), "[lyra] commands: /exit  /help  /new  /plan <msg>  /session")
	case "/new":
		sess, err := rt.session.Create(context.Background(), "")
		if err != nil {
			fmt.Fprintf(stderr(), "[lyra] create session: %s\n", err)
			return false, ""
		}
		fmt.Fprintf(stderr(), "[lyra] switched to fresh session %s\n", sess.ID)
		return false, sess.ID
	case "/session":
		fmt.Fprintf(stderr(), "[lyra] current session: %s\n", current)
	default:
		fmt.Fprintf(stderr(), "[lyra] unknown command %q (try /help)\n", line)
	}
	return false, ""
}

