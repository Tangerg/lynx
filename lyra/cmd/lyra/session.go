package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
)

// cmdSession is `lyra session <sub>` — list / show / delete the
// runtime's known sessions. SessionService is in-memory only today;
// a `lyra session list` after a process restart returns an empty
// slice until M3 persistence lands.
func cmdSession(args []string) int {
	if len(args) == 0 {
		printSessionUsage()
		return 2
	}
	sub, rest := args[0], args[1:]

	rt, err := newRuntime()
	if err != nil {
		return printErr(err)
	}

	switch sub {
	case "list", "ls":
		return sessionList(rt)
	case "show":
		return sessionShow(rt, rest)
	case "delete", "rm":
		return sessionDelete(rt, rest)
	case "-h", "--help", "help":
		printSessionUsage()
		return 0
	default:
		fmt.Fprintf(stderr(), "lyra session: unknown sub-command %q\n\n", sub)
		printSessionUsage()
		return 2
	}
}

func printSessionUsage() {
	fmt.Fprintln(stderr(), "Usage: lyra session <list|show|delete> [args]")
	fmt.Fprintln(stderr(), "  list           List all known sessions.")
	fmt.Fprintln(stderr(), "  show <id>      Show one session's metadata.")
	fmt.Fprintln(stderr(), "  delete <id>    Drop a session (idempotent).")
}

func sessionList(rt *runtime) int {
	sessions, err := rt.session.List(context.Background())
	if err != nil {
		return printErr(err)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(stdout(), "(no sessions)")
		return 0
	}
	// Plain table; tabwriter would be cleaner but std-only keeps deps small.
	fmt.Fprintf(stdout(), "%-36s  %-19s  %5s  %s\n", "ID", "UPDATED", "TURNS", "TITLE")
	for _, s := range sessions {
		fmt.Fprintf(stdout(), "%-36s  %-19s  %5d  %s\n",
			s.ID,
			s.UpdatedAt.Format("2006-01-02 15:04:05"),
			s.TurnCount,
			s.Title,
		)
	}
	return 0
}

func sessionShow(rt *runtime, args []string) int {
	fs := newSubFlagSet("session show")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr(), "Usage: lyra session show <id>")
		return 2
	}
	id := fs.Arg(0)

	sess, err := rt.session.Get(context.Background(), id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			fmt.Fprintf(stderr(), "lyra: session %q not found\n", id)
			return 1
		}
		return printErr(err)
	}
	fmt.Fprintf(stdout(), "id:         %s\n", sess.ID)
	fmt.Fprintf(stdout(), "title:      %s\n", sess.Title)
	if sess.ParentID != "" {
		fmt.Fprintf(stdout(), "parent:     %s\n", sess.ParentID)
	}
	fmt.Fprintf(stdout(), "started_at: %s\n", sess.StartedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(stdout(), "updated_at: %s\n", sess.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(stdout(), "turns:      %d\n", sess.TurnCount)
	for k, v := range sess.Metadata {
		fmt.Fprintf(stdout(), "meta.%s: %s\n", k, v)
	}
	return 0
}

func sessionDelete(rt *runtime, args []string) int {
	fs := newSubFlagSet("session delete")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr(), "Usage: lyra session delete <id>")
		return 2
	}
	if err := rt.session.Delete(context.Background(), fs.Arg(0)); err != nil {
		return printErr(err)
	}
	fmt.Fprintf(stderr(), "[lyra] deleted session %s\n", fs.Arg(0))
	return 0
}
