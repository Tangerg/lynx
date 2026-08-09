package cmd

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newApprovalsCommand(resolve backend) *cobra.Command {
	command := &cobra.Command{Use: "approvals", Short: "Inspect remembered approval rules", Args: cobra.NoArgs}
	command.AddCommand(&cobra.Command{
		Use:          "ls",
		Aliases:      []string{"list"},
		Short:        "List remembered approval rules",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := resolve(cmd)
			if err != nil {
				return err
			}
			rules, err := runtime.ListApprovalRules(cmd.Context())
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			for _, rule := range rules {
				target := rule.Workspace
				if target == "" {
					target = rule.SessionID
				}
				if target == "" {
					target = "*"
				}
				if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", rule.ID, rule.Scope, rule.Decision, target, rule.Rule); err != nil {
					return err
				}
			}
			return writer.Flush()
		},
	})
	var yes bool
	remove := &cobra.Command{
		Use:          "delete <rule-id>",
		Aliases:      []string{"rm", "forget"},
		Short:        "Forget a remembered approval rule",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("refusing to delete an approval rule without --yes")
			}
			runtime, err := resolve(cmd)
			if err != nil {
				return err
			}
			if err := runtime.DeleteApprovalRule(cmd.Context(), args[0]); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), args[0])
			return err
		},
	}
	remove.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm forgetting the rule")
	command.AddCommand(remove)
	return command
}
