package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// SessionCmd is the `lyra session` group — CRUD over saved
// sessions. Subcommands attach as cobra children so `lyra
// session --help` lists them automatically.
func (a *App) SessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage saved sessions (list / show / delete).",
	}
	cmd.AddCommand(
		a.sessionListCmd(),
		a.sessionShowCmd(),
		a.sessionDeleteCmd(),
	)
	return cmd
}

func (a *App) sessionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all known sessions.",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensureRuntime(cmd.Context()); err != nil {
				return a.fatalErr(err)
			}
			sessions, err := a.rt.Session().List(cmd.Context())
			if err != nil {
				return a.fatalErr(err)
			}
			if len(sessions) == 0 {
				fmt.Fprintln(a.Out, "(no sessions)")
				return nil
			}
			fmt.Fprintf(a.Out, "%-36s  %-19s  %s\n", "ID", "UPDATED", "TITLE")
			for _, s := range sessions {
				fmt.Fprintf(a.Out, "%-36s  %-19s  %s\n",
					s.ID,
					s.UpdatedAt.Format("2006-01-02 15:04:05"),
					s.Title,
				)
			}
			return nil
		},
	}
}

func (a *App) sessionShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show one session's metadata.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensureRuntime(cmd.Context()); err != nil {
				return a.fatalErr(err)
			}
			id := args[0]
			sess, err := a.rt.Session().Get(cmd.Context(), id)
			if err != nil {
				if errors.Is(err, session.ErrNotFound) {
					fmt.Fprintf(a.Err, "lyra: session %q not found\n", id)
					return errSilenced
				}
				return a.fatalErr(err)
			}
			printSession(a.Out, sess)
			return nil
		},
	}
}

func (a *App) sessionDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"rm"},
		Short:   "Drop a session (idempotent).",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensureRuntime(cmd.Context()); err != nil {
				return a.fatalErr(err)
			}
			if err := a.rt.Session().Delete(cmd.Context(), args[0]); err != nil {
				return a.fatalErr(err)
			}
			fmt.Fprintf(a.Err, "[lyra] deleted session %s\n", args[0])
			return nil
		},
	}
}

// printSession is the shared renderer for `session show`. Kept as
// a package-level function (not a method) because it depends on
// nothing but the supplied writer + session value.
func printSession(out io.Writer, sess session.Session) {
	fmt.Fprintf(out, "id:         %s\n", sess.ID)
	fmt.Fprintf(out, "title:      %s\n", sess.Title)
	if sess.ParentID != "" {
		fmt.Fprintf(out, "parent:     %s\n", sess.ParentID)
	}
	fmt.Fprintf(out, "started_at: %s\n", sess.StartedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(out, "updated_at: %s\n", sess.UpdatedAt.Format("2006-01-02 15:04:05"))
	for k, v := range sess.Metadata {
		fmt.Fprintf(out, "meta.%s: %s\n", k, v)
	}
}
