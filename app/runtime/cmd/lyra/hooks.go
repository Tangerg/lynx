package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	adapterhooks "github.com/Tangerg/lynx/app/runtime/internal/adapter/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
)

// HooksCmd is the `lyra hooks` group — manage which projects are trusted to run
// their .lyra/hooks.json lifecycle hooks. Global hooks (~/.lyra/hooks.json) are
// always trusted (they're the user's own); a project's hooks stay inert (a
// cloned repo must not auto-run code) until trusted here. No API key needed —
// these only touch the local trust store, so they don't build the runtime.
func (a *App) HooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Manage project hook trust (trust / untrust / list).",
		Long: "Lifecycle hooks (PreToolUse, PostToolUse, …) configured in a project's " +
			".lyra/hooks.json run only after you trust the project — a cloned repo's hooks " +
			"never auto-execute. Global hooks in ~/.lyra/hooks.json are always trusted.",
	}
	cmd.AddCommand(a.hooksTrustCmd(), a.hooksUntrustCmd(), a.hooksListCmd())
	return cmd
}

// projectRootArg resolves the project root from an optional dir arg (default:
// the cwd), so `lyra hooks trust` with no arg trusts the current project.
func projectRootArg(args []string) (string, error) {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return adapterhooks.ProjectRoot(abs), nil
}

func (a *App) hooksTrustCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trust [dir]",
		Short: "Trust a project's .lyra/hooks.json (default: current project).",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := projectRootArg(args)
			if err != nil {
				return a.fatalErr(err)
			}
			stores, err := persistence.Open()
			if err != nil {
				return a.fatalErr(err)
			}
			if err := stores.Trust.Trust(cmd.Context(), root); err != nil {
				return a.fatalErr(err)
			}
			fmt.Fprintf(a.Out, "trusted hooks for %s\n", root)
			return nil
		},
	}
}

func (a *App) hooksUntrustCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "untrust [dir]",
		Short: "Revoke hook trust for a project (default: current project).",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := projectRootArg(args)
			if err != nil {
				return a.fatalErr(err)
			}
			stores, err := persistence.Open()
			if err != nil {
				return a.fatalErr(err)
			}
			if err := stores.Trust.Untrust(cmd.Context(), root); err != nil {
				return a.fatalErr(err)
			}
			fmt.Fprintf(a.Out, "revoked hook trust for %s\n", root)
			return nil
		},
	}
}

func (a *App) hooksListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List trusted projects.",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			stores, err := persistence.Open()
			if err != nil {
				return a.fatalErr(err)
			}
			roots, err := stores.Trust.List(cmd.Context())
			if err != nil {
				return a.fatalErr(err)
			}
			if len(roots) == 0 {
				fmt.Fprintln(a.Out, "(no trusted projects — only ~/.lyra/hooks.json runs)")
				return nil
			}
			for _, r := range roots {
				fmt.Fprintln(a.Out, r)
			}
			return nil
		},
	}
}
