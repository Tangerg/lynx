package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/render"
)

func newSessionsCommand(resolve backend) *cobra.Command {
	sessions := &cobra.Command{
		Use:     "sessions",
		Short:   "Inspect and manage sessions",
		Aliases: []string{"session"},
		Args:    cobra.NoArgs,
	}
	sessions.AddCommand(
		newSessionsListCommand(resolve),
		newSessionsShowCommand(resolve),
		newSessionsRenameCommand(resolve),
		newSessionsForkCommand(resolve),
		newSessionsDeleteCommand(resolve),
	)
	return sessions
}

func newSessionsListCommand(resolve backend) *cobra.Command {
	var query client.SessionQuery
	cmd := &cobra.Command{
		Use:          "ls",
		Short:        "List sessions, most recently touched first",
		Aliases:      []string{"list"},
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := resolve.open(cmd)
			if err != nil {
				return err
			}
			page, err := runtime.ListSessions(cmd.Context(), query)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			for _, session := range page.Items {
				title := session.Title
				if title == "" {
					title = "(untitled)"
				}
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", session.ID, ago(session.UpdatedAt), title, session.Workspace); err != nil {
					return err
				}
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if page.NextCursor != "" {
				_, err = fmt.Fprintf(cmd.ErrOrStderr(), "more sessions: --cursor %s\n", page.NextCursor)
			}
			return err
		},
	}
	cmd.Flags().IntVarP(&query.Limit, "limit", "n", 20, "Maximum sessions to return (up to 100)")
	cmd.Flags().StringVar(&query.Cursor, "cursor", "", "Opaque cursor returned by the previous page")
	cmd.Flags().StringVarP(&query.Search, "search", "q", "", "Search session titles and workspaces")
	cmd.Flags().StringVar(&query.Workspace, "workspace", "", "Only sessions in this exact workspace")
	return cmd
}

func newSessionsShowCommand(resolve backend) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:          "show <session-id>",
		Short:        "Print a saved session transcript",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := resolve.open(cmd)
			if err != nil {
				return err
			}
			snapshot, err := runtime.GetSession(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			var output renderer = render.NewText(cmd.OutOrStdout())
			if asJSON {
				output = render.NewJSON(cmd.OutOrStdout())
			} else if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s · %s\n", snapshot.Session.Title, snapshot.Session.Workspace); err != nil {
				return err
			}
			for _, envelope := range snapshot.Events {
				if err := output.Render(envelope); err != nil {
					return err
				}
			}
			return output.Close()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Write newline-delimited event JSON")
	cmd.ValidArgsFunction = completeSessionIDs(resolve)
	return cmd
}

func newSessionsRenameCommand(resolve backend) *cobra.Command {
	var revision int64
	cmd := &cobra.Command{
		Use:          "rename <session-id> <title>",
		Short:        "Rename a session",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := resolve.open(cmd)
			if err != nil {
				return err
			}
			updated, err := runtime.UpdateSession(cmd.Context(), client.UpdateSession{SessionID: args[0], Title: args[1], Revision: revision})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\t%s\n", updated.ID, updated.Revision, updated.Title)
			return err
		},
	}
	cmd.Flags().Int64Var(&revision, "revision", 0, "Only rename the revision previously read")
	cmd.ValidArgsFunction = completeFirstSessionArgument(resolve)
	return cmd
}

func newSessionsForkCommand(resolve backend) *cobra.Command {
	var (
		at    uint64
		title string
	)
	cmd := &cobra.Command{
		Use:          "fork <session-id>",
		Short:        "Fork a session from a transcript cursor",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := resolve.open(cmd)
			if err != nil {
				return err
			}
			forked, err := runtime.ForkSession(cmd.Context(), client.ForkSession{SessionID: args[0], At: client.Cursor(at), Title: title})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), forked.ID)
			return err
		},
	}
	cmd.Flags().Uint64Var(&at, "at", 0, "Fork through this event cursor (default: latest)")
	cmd.Flags().StringVar(&title, "title", "", "Title for the fork")
	cmd.ValidArgsFunction = completeSessionIDs(resolve)
	return cmd
}

func newSessionsDeleteCommand(resolve backend) *cobra.Command {
	var (
		revision int64
		yes      bool
	)
	cmd := &cobra.Command{
		Use:          "delete <session-id>",
		Short:        "Delete a session",
		Aliases:      []string{"rm"},
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("refusing to delete without --yes")
			}
			runtime, err := resolve.open(cmd)
			if err != nil {
				return err
			}
			if err := runtime.DeleteSession(cmd.Context(), client.DeleteSession{SessionID: args[0], Revision: revision}); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), args[0])
			return err
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm deletion")
	cmd.Flags().Int64Var(&revision, "revision", 0, "Only delete the revision previously read")
	cmd.ValidArgsFunction = completeSessionIDs(resolve)
	return cmd
}

func completeFirstSessionArgument(resolve backend) cobra.CompletionFunc {
	complete := completeSessionIDs(resolve)
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return complete(cmd, args, toComplete)
	}
}

func completeSessionIDs(resolve backend) cobra.CompletionFunc {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		runtime, err := resolve.forCompletion(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		page, err := runtime.ListSessions(cmd.Context(), client.SessionQuery{Limit: 100, Search: toComplete})
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		items := make([]string, 0, len(page.Items))
		for _, session := range page.Items {
			if toComplete == "" || strings.HasPrefix(session.ID, toComplete) || strings.Contains(strings.ToLower(session.Title), strings.ToLower(toComplete)) {
				items = append(items, session.ID+"\t"+session.Title)
			}
		}
		return items, cobra.ShellCompDirectiveNoFileComp
	}
}

// ago phrases a timestamp the way a session list is read.
func ago(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	}
}
