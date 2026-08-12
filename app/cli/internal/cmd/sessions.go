package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/render"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/session"
)

func newSessionsCommand(provider runtimeProvider) *cobra.Command {
	sessions := &cobra.Command{
		Use:     "sessions",
		Short:   "Inspect and manage sessions",
		Aliases: []string{"session"},
		Args:    cobra.NoArgs,
	}
	sessions.AddCommand(
		newSessionsListCommand(provider),
		newSessionsShowCommand(provider),
		newSessionsUpdateCommand(provider),
		newSessionsRenameCommand(provider),
		newSessionsForkCommand(provider),
		newSessionsDeleteCommand(provider),
	)
	return sessions
}

func newSessionsUpdateCommand(provider runtimeProvider) *cobra.Command {
	var (
		title, workspace, model string
		favorite                bool
		revision                uint64
	)
	cmd := &cobra.Command{
		Use:          "update <session-id>",
		Short:        "Update session metadata, model, or workspace",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			update := agent.UpdateSession{SessionID: args[0], ExpectedRevision: revision}
			if cmd.Flags().Changed("title") {
				update.Title = &title
			}
			if cmd.Flags().Changed("workspace") {
				resolved, err := canonicalWorkspacePath(workspace)
				if err != nil {
					return err
				}
				update.Workspace = &resolved
			}
			if cmd.Flags().Changed("model") {
				update.Model = &model
			}
			if cmd.Flags().Changed("favorite") {
				update.Favorite = &favorite
			}
			if err := update.Validate(); err != nil {
				return err
			}
			services, err := provider.OpenServices(cmd)
			if err != nil {
				return err
			}
			if update.Workspace != nil && services.RuntimeProfile != nil &&
				!services.RuntimeProfile.Supports(runtimeprofile.FeatureRelocate) {
				return fmt.Errorf("runtime capability %q was not negotiated", runtimeprofile.FeatureRelocate)
			}
			updated, err := session.Update(cmd.Context(), services.Agent, update)
			if err != nil {
				return err
			}
			return render.WriteSessionJSON(cmd.OutOrStdout(), updated)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Replace the session title")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Relocate the session to this workspace")
	cmd.Flags().StringVar(&model, "model", "", "Set the session model (empty selects the runtime default)")
	cmd.Flags().BoolVar(&favorite, "favorite", false, "Set whether the session is a favorite")
	cmd.Flags().Uint64Var(&revision, "revision", 0, "Revision previously read from sessions ls/show")
	_ = cmd.MarkFlagRequired("revision")
	cmd.ValidArgsFunction = completeFirstSessionArgument(provider)
	return cmd
}

func newSessionsListCommand(provider runtimeProvider) *cobra.Command {
	var (
		query  agent.SessionQuery
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:          "ls",
		Short:        "List sessions, most recently touched first",
		Aliases:      []string{"list"},
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if query.Workspace != "" {
				resolved, err := canonicalWorkspacePath(query.Workspace)
				if err != nil {
					return err
				}
				query.Workspace = resolved
			}
			runtime, err := provider.Open(cmd)
			if err != nil {
				return err
			}
			page, err := runtime.ListSessions(cmd.Context(), query)
			if err != nil {
				return err
			}
			if err := page.Validate(); err != nil {
				return fmt.Errorf("list sessions: %w", err)
			}
			if asJSON {
				return render.WriteSessionPageJSON(cmd.OutOrStdout(), page)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			for _, session := range page.Items {
				title := session.Title
				if title == "" {
					title = "(untitled)"
				}
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", session.ID, relativeAge(session.UpdatedAt), title, sessionWorkspaceLabel(session)); err != nil {
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
	cmd.Flags().BoolVar(&asJSON, "json", false, "Write the page as JSON")
	return cmd
}

func sessionWorkspaceLabel(session agent.Session) string {
	if session.Workspace.IsAvailable() {
		return session.Workspace.Path
	}
	return session.Workspace.Path + " (missing)"
}

func newSessionsShowCommand(provider runtimeProvider) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:          "show <session-id>",
		Short:        "Print a saved session transcript",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := provider.Open(cmd)
			if err != nil {
				return err
			}
			snapshot, err := runtime.GetSession(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := snapshot.Validate(); err != nil {
				return fmt.Errorf("show session: %w", err)
			}
			return writeSessionSnapshot(cmd, snapshot, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Write the authoritative cold snapshot as JSON")
	cmd.ValidArgsFunction = completeSessionIDs(provider)
	return cmd
}

func writeSessionSnapshot(cmd *cobra.Command, snapshot agent.SessionSnapshot, asJSON bool) error {
	if asJSON {
		return render.WriteSessionSnapshotJSON(cmd.OutOrStdout(), snapshot)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s · %s\n", snapshot.Session.Title, sessionWorkspaceLabel(snapshot.Session)); err != nil {
		return err
	}
	return render.WriteSessionTranscript(cmd.OutOrStdout(), snapshot)
}

func newSessionsRenameCommand(provider runtimeProvider) *cobra.Command {
	var revision uint64
	cmd := &cobra.Command{
		Use:          "rename <session-id> <title>",
		Short:        "Rename a session",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := provider.Open(cmd)
			if err != nil {
				return err
			}
			title := args[1]
			updated, err := session.Update(cmd.Context(), runtime, agent.UpdateSession{
				SessionID: args[0], Title: &title, ExpectedRevision: revision,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\t%s\n", updated.ID, updated.Revision, updated.Title)
			return err
		},
	}
	cmd.Flags().Uint64Var(&revision, "revision", 0, "Revision previously read from sessions ls/show")
	_ = cmd.MarkFlagRequired("revision")
	cmd.ValidArgsFunction = completeFirstSessionArgument(provider)
	return cmd
}

func newSessionsForkCommand(provider runtimeProvider) *cobra.Command {
	var (
		fromRun string
		title   string
	)
	cmd := &cobra.Command{
		Use:          "fork <session-id>",
		Short:        "Fork a session at a run boundary",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := provider.Open(cmd)
			if err != nil {
				return err
			}
			forked, err := runtime.ForkSession(cmd.Context(), agent.ForkSession{SessionID: args[0], FromRunID: fromRun, Title: title})
			if err != nil {
				return err
			}
			if err := forked.Validate(); err != nil {
				return fmt.Errorf("fork session: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), forked.ID)
			return err
		},
	}
	cmd.Flags().StringVar(&fromRun, "from-run", "", "Fork through this root run (default: complete session)")
	cmd.Flags().StringVar(&title, "title", "", "Title for the fork")
	cmd.ValidArgsFunction = completeSessionIDs(provider)
	return cmd
}

func newSessionsDeleteCommand(provider runtimeProvider) *cobra.Command {
	var (
		yes bool
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
			runtime, err := provider.Open(cmd)
			if err != nil {
				return err
			}
			if err := runtime.DeleteSession(cmd.Context(), agent.DeleteSession{SessionID: args[0]}); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), args[0])
			return err
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm deletion")
	cmd.ValidArgsFunction = completeSessionIDs(provider)
	return cmd
}

func completeFirstSessionArgument(provider runtimeProvider) cobra.CompletionFunc {
	complete := completeSessionIDs(provider)
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return complete(cmd, args, toComplete)
	}
}

func completeSessionIDs(provider runtimeProvider) cobra.CompletionFunc {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		runtime, err := provider.OpenQuietly(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		page, err := runtime.ListSessions(cmd.Context(), agent.SessionQuery{Limit: 100, Search: toComplete})
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		if err := page.Validate(); err != nil {
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

// relativeAge phrases a timestamp the way a session list is read.
func relativeAge(t time.Time) string {
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
