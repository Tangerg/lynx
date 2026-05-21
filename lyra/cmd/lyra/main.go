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

// commands is the dispatch table. Keep entries alphabetical so the
// help listing is predictable.
var commands = map[string]struct {
	run     subcommand
	summary string
}{
	"chat":    {run: cmdChat, summary: "Send one message and print the streamed reply."},
	"repl":    {run: cmdRepl, summary: "Interactive multi-turn conversation."},
	"session": {run: cmdSession, summary: "Manage saved sessions (list / show / delete)."},
	"version": {run: cmdVersion, summary: "Print build / version info."},
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
	cmd, ok := commands[name]
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
	// Stable ordering: same names walk in the same order each run.
	for _, name := range commandOrder() {
		fmt.Fprintf(w, "  %-8s  %s\n", name, commands[name].summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `lyra <command> -h` for command-specific help.")
}

func commandOrder() []string {
	return []string{"chat", "repl", "session", "version"}
}
