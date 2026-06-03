package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/lyra/internal/service/agentdoc"
)

// AgentsCmd is `lyra agents` — print the AGENTS.md files Lyra would
// inject into the system prompt at the current cwd.
//
// Walks the same paths the engine uses (user-level + project tree
// up to .git root), so users can verify what context the model
// actually sees before kicking off a turn. Doesn't require an API
// key — pure filesystem operation.
func (a *App) AgentsCmd() *cobra.Command {
	var showContent bool
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "List AGENTS.md files Lyra will inject into the system prompt.",
		Long: `List the AGENTS.md files the engine discovers for the current
working directory. Walks from cwd up to the project root (nearest
.git ancestor) plus the standard user-level paths:

  ~/.lyra/AGENTS.md          (Lyra-specific user scope)
  ~/.agents/AGENTS.md        (cross-tool generic user scope)
  {dir}/.lyra/AGENTS.md      (Lyra subdir convention at every level)
  {dir}/AGENTS.md            (cross-tool convention at every level)

Render order is user-scope first, then project root → leaf, so
deeper files end up closer to the model's most-recent context.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return a.fatalErr(err)
			}
			home, _ := os.UserHomeDir()

			files, err := agentdoc.Discover(cmd.Context(), cwd, home)
			if err != nil {
				return a.fatalErr(err)
			}
			if len(files) == 0 {
				fmt.Fprintln(a.Out, "(no AGENTS.md found)")
				return nil
			}

			for _, f := range files {
				fmt.Fprintf(a.Out, "%s (%d bytes)\n", f.Path, len(f.Content))
				if showContent {
					fmt.Fprintln(a.Out, "---")
					fmt.Fprintln(a.Out, f.Content)
					fmt.Fprintln(a.Out, "---")
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&showContent, "show", false, "print each file's content beneath its path")
	return cmd
}
