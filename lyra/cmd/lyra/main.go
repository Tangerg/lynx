// Command lyra is the entrypoint for the Lyra general-purpose agent
// runtime. It dispatches on the first argument to one of the
// subcommands below; each subcommand owns its own flag parsing.
//
// Layout — one file per subcommand keeps each entry self-contained:
//
//	main.go     — argv dispatch + shared wiring (newRuntime)
//	chat.go     — `lyra chat <message>` one-shot
//	repl.go     — `lyra repl [--session ID]` interactive
//	session.go  — `lyra session {list|show|delete}` management
//	memory.go   — `lyra memory {show|set|clear}` LYRA.md cascade
//	version.go  — `lyra version` build info
//
// Transport layer (HTTP/gRPC/IPC) is *not* in this CLI — these
// subcommands consume the in-process Service interfaces directly.
// Phase 2 wraps the same services behind transport adapters.
package main

import (
	"fmt"
	"os"
)

// subcommand is the contract every CLI sub-binary implements. Each
// receives its own argv slice (post-subcommand) and is responsible
// for parsing its own flags. exit code surfaces through the int
// return — non-zero propagates to os.Exit.
type subcommand func(args []string) int

// command is one row of the dispatch table. The slice in [commands]
// is the single source of truth for both name → handler dispatch
// (via [lookupCommand]) and the help-listing order.
type command struct {
	name    string
	run     subcommand
	summary string
}

// commands lists every subcommand in the order help renders them.
// Slice (not map) so order is intrinsic — adding a new entry is a
// single-line change, no second list to keep in sync.
var commands = []command{
	{"chat", cmdChat, "Send one message and print the streamed reply."},
	{"repl", cmdRepl, "Interactive multi-turn conversation."},
	{"memory", cmdMemory, "Inspect / edit the LYRA.md memory cascade."},
	{"session", cmdSession, "Manage saved sessions (list / show / delete)."},
	{"version", cmdVersion, "Print build / version info."},
}

// lookupCommand finds the command entry matching name, or (nil, false).
// Linear scan is fine — the command list is tiny and bounded.
func lookupCommand(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		printUsage(os.Stdout)
		if len(os.Args) < 2 {
			os.Exit(2)
		}
		return
	}

	name := os.Args[1]
	cmd, ok := lookupCommand(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "lyra: unknown command %q\n\n", name)
		printUsage(os.Stderr)
		os.Exit(2)
	}

	os.Exit(cmd.run(os.Args[2:]))
}

// printUsage writes the top-level command listing. Subcommand-
// specific usage lives on each cmd* function (they print their own
// when called with -h).
func printUsage(w *os.File) {
	fmt.Fprintln(w, "lyra — general-purpose agent runtime")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage: lyra <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-8s  %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `lyra <command> -h` for command-specific help.")
}
