package cmd

import (
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func newSessionsCommand(resolve backend) *cobra.Command {
	sessions := &cobra.Command{
		Use:     "sessions",
		Short:   "Inspect sessions",
		Aliases: []string{"session"},
	}
	sessions.AddCommand(newSessionsListCommand(resolve))
	return sessions
}

func newSessionsListCommand(resolve backend) *cobra.Command {
	return &cobra.Command{
		Use:          "ls",
		Short:        "List sessions, most recently touched first",
		Aliases:      []string{"list"},
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := resolve(cmd)
			if err != nil {
				return err
			}
			list, err := rt.ListSessions(cmd.Context())
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			for _, s := range list {
				title := s.Title
				if title == "" {
					title = "(untitled)"
				}
				if _, err := w.Write([]byte(s.ID + "\t" + ago(s.UpdatedAt) + "\t" + title + "\t" + s.Workspace + "\n")); err != nil {
					return err
				}
			}
			return w.Flush()
		},
	}
}

// ago phrases a timestamp the way a session list is read — how long since it was
// touched, not when in absolute terms.
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
