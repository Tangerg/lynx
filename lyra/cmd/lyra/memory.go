package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/lyra/internal/service/memory"
)

// MemoryCmd is the `lyra memory` group — inspect / edit the
// LYRA.md cascade the agent injects into every system prompt.
// Two scopes:
//
//   - project   <cwd>/LYRA.md      (per-repo knowledge)
//   - user      ~/.lyra/LYRA.md    (cross-project preferences)
func (a *App) MemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Inspect / edit the LYRA.md memory cascade.",
	}
	cmd.AddCommand(
		a.memoryShowCmd(),
		a.memorySetCmd(),
		a.memoryClearCmd(),
	)
	return cmd
}

func (a *App) memoryShowCmd() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the LYRA.md cascade (default: both scopes).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensureRuntime(cmd.Context()); err != nil {
				return a.fatalErr(err)
			}
			target, both, err := parseScope(scope, true)
			if err != nil {
				return a.fatalErr(err)
			}
			ctx := cmd.Context()
			if both {
				a.printScope(ctx, memory.ScopeUser, "user")
				a.printScope(ctx, memory.ScopeProject, "project")
				return nil
			}
			a.printScope(ctx, target, scope)
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "both", "memory scope: project | user | both")
	return cmd
}

func (a *App) memorySetCmd() *cobra.Command {
	var (
		scope string
		from  string
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Overwrite a scope from stdin or --from <path>.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensureRuntime(cmd.Context()); err != nil {
				return a.fatalErr(err)
			}
			target, _, err := parseScope(scope, false)
			if err != nil {
				return a.fatalErr(err)
			}
			content, err := readMemoryBody(from, a.In)
			if err != nil {
				return a.fatalErr(err)
			}
			if err := a.rt.Memory().Update(cmd.Context(), target, string(content)); err != nil {
				return a.fatalErr(err)
			}
			fmt.Fprintf(a.Err, "[lyra] %s memory updated (%d bytes)\n", scope, len(content))
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "project", "memory scope: project | user")
	cmd.Flags().StringVar(&from, "from", "", "read content from this file (default: stdin)")
	return cmd
}

func (a *App) memoryClearCmd() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Empty a scope.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensureRuntime(cmd.Context()); err != nil {
				return a.fatalErr(err)
			}
			target, _, err := parseScope(scope, false)
			if err != nil {
				return a.fatalErr(err)
			}
			if err := a.rt.Memory().Update(cmd.Context(), target, ""); err != nil {
				return a.fatalErr(err)
			}
			fmt.Fprintf(a.Err, "[lyra] %s memory cleared\n", scope)
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "project", "memory scope: project | user")
	return cmd
}

// parseScope maps the --scope flag value to the typed enum.
// allowBoth controls whether "both" is acceptable — show accepts
// it, set / clear don't.
func parseScope(s string, allowBoth bool) (target memory.Scope, both bool, err error) {
	switch s {
	case "project":
		return memory.ScopeProject, false, nil
	case "user":
		return memory.ScopeUser, false, nil
	case "both":
		if !allowBoth {
			return 0, false, fmt.Errorf("scope %q not allowed here (use project|user)", s)
		}
		return 0, true, nil
	}
	allowed := "project | user"
	if allowBoth {
		allowed += " | both"
	}
	return 0, false, fmt.Errorf("scope must be one of %s", allowed)
}

// readMemoryBody returns the content to write — from the named
// file when from != "", otherwise drained from stdin.
func readMemoryBody(from string, stdin io.Reader) ([]byte, error) {
	if from != "" {
		return os.ReadFile(from)
	}
	return io.ReadAll(stdin)
}

// printScope writes one scope's contents to the App's stdout
// under a markdown heading. Errors print to stderr but don't
// abort the show command — partial output is still useful.
func (a *App) printScope(ctx context.Context, scope memory.Scope, label string) {
	content, err := a.rt.Memory().Get(ctx, scope)
	if err != nil {
		fmt.Fprintf(a.Err, "[lyra] %s scope read error: %s\n", label, err)
		return
	}
	fmt.Fprintf(a.Out, "## %s\n", label)
	if content == "" {
		fmt.Fprintln(a.Out, "(empty)")
	} else {
		fmt.Fprintln(a.Out, content)
	}
	fmt.Fprintln(a.Out)
}
