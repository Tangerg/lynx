package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/service/memory"
)

// cmdMemory is `lyra memory <sub>` — inspect and edit the LYRA.md
// cascade the agent injects into every system prompt. Two scopes:
//
//   - project   <cwd>/LYRA.md          (per-repo knowledge)
//   - user      ~/.lyra/LYRA.md        (cross-project preferences)
//
// `lyra memory show` prints both; `set` overwrites one scope from
// stdin or from a file flag; `clear` empties one scope.
func cmdMemory(args []string) int {
	if len(args) == 0 {
		printMemoryUsage()
		return 2
	}
	sub, rest := args[0], args[1:]

	// Cheap help paths don't need the runtime (no API key required).
	switch sub {
	case "-h", "--help", "help":
		printMemoryUsage()
		return 0
	}

	rt, err := newRuntime()
	if err != nil {
		return printErr(err)
	}

	switch sub {
	case "show":
		return memoryShow(rt, rest)
	case "set":
		return memorySet(rt, rest)
	case "clear":
		return memoryClear(rt, rest)
	default:
		fmt.Fprintf(stderr(), "lyra memory: unknown sub-command %q\n\n", sub)
		printMemoryUsage()
		return 2
	}
}

func printMemoryUsage() {
	fmt.Fprintln(stderr(), "Usage: lyra memory <show|set|clear> [--scope project|user] [--from path]")
	fmt.Fprintln(stderr(), "  show              Print the LYRA.md cascade (default: both scopes).")
	fmt.Fprintln(stderr(), "  set --scope X     Overwrite scope X from stdin or --from <path>.")
	fmt.Fprintln(stderr(), "  clear --scope X   Empty scope X.")
}

// scopeFlag attaches a --scope option to fs. When allowBoth is
// true "both" becomes the default + an accepted value; otherwise
// scope is restricted to project / user. Returned pointer's
// resolve() turns the parsed value into the typed
// (memory.Scope, both bool).
func scopeFlag(fs *flag.FlagSet, allowBoth bool) *memoryScopeFlag {
	v := &memoryScopeFlag{value: "project", allowBoth: allowBoth}
	help := "memory scope: project | user"
	if allowBoth {
		v.value = "both"
		help += " | both"
	}
	fs.Var(v, "scope", help)
	return v
}

type memoryScopeFlag struct {
	value     string
	allowBoth bool
}

// validScopes lists the textual scope values accepted by --scope.
// Restricted by [memoryScopeFlag.allowBoth] — "both" only shows
// up when the caller of [scopeFlag] opted in.
func (f *memoryScopeFlag) validScopes() []string {
	if f.allowBoth {
		return []string{"project", "user", "both"}
	}
	return []string{"project", "user"}
}

func (f *memoryScopeFlag) String() string { return f.value }

func (f *memoryScopeFlag) Set(s string) error {
	for _, v := range f.validScopes() {
		if v == s {
			f.value = s
			return nil
		}
	}
	return fmt.Errorf("scope must be one of %s", strings.Join(f.validScopes(), " | "))
}

// resolve maps the parsed value into the runtime types. Returns
// (target, both): both=true means "operate on every scope" — only
// valid when the flag was constructed with allowBoth.
func (f *memoryScopeFlag) resolve() (memory.Scope, bool) {
	switch f.value {
	case "user":
		return memory.ScopeUser, false
	case "both":
		return 0, true
	}
	// project is the default + the fallback for unrecognised
	// values (which Set would already have rejected).
	return memory.ScopeProject, false
}

func memoryShow(rt *runtime, args []string) int {
	fs := newSubFlagSet("memory show")
	scope := scopeFlag(fs, true)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx := context.Background()
	target, both := scope.resolve()
	if both {
		printScope(rt, ctx, memory.ScopeUser, "user")
		printScope(rt, ctx, memory.ScopeProject, "project")
		return 0
	}
	printScope(rt, ctx, target, scope.value)
	return 0
}

func printScope(rt *runtime, ctx context.Context, scope memory.Scope, label string) {
	content, err := rt.memory.Get(ctx, scope)
	if err != nil {
		fmt.Fprintf(stderr(), "[lyra] %s scope read error: %s\n", label, err)
		return
	}
	fmt.Fprintf(stdout(), "## %s\n", label)
	if content == "" {
		fmt.Fprintln(stdout(), "(empty)")
	} else {
		fmt.Fprintln(stdout(), content)
	}
	fmt.Fprintln(stdout())
}

func memorySet(rt *runtime, args []string) int {
	fs := newSubFlagSet("memory set")
	scope := scopeFlag(fs, false)
	from := fs.String("from", "", "read content from this file (default: stdin)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var content []byte
	var err error
	if *from != "" {
		content, err = os.ReadFile(*from)
		if err != nil {
			return printErr(err)
		}
	} else {
		content, err = io.ReadAll(stdin())
		if err != nil {
			return printErr(err)
		}
	}

	target, _ := scope.resolve()
	if err := rt.memory.Update(context.Background(), target, string(content)); err != nil {
		return printErr(err)
	}
	fmt.Fprintf(stderr(), "[lyra] %s memory updated (%d bytes)\n", scope.value, len(content))
	return 0
}

func memoryClear(rt *runtime, args []string) int {
	fs := newSubFlagSet("memory clear")
	scope := scopeFlag(fs, false)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	target, _ := scope.resolve()
	if err := rt.memory.Update(context.Background(), target, ""); err != nil {
		return printErr(err)
	}
	fmt.Fprintf(stderr(), "[lyra] %s memory cleared\n", scope.value)
	return 0
}
