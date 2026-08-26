package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func newApprovalsCommand(provider runtimeProvider) *cobra.Command {
	command := &cobra.Command{Use: "approvals", Short: "Inspect remembered approval rules", Args: cobra.NoArgs}
	var (
		asJSON        bool
		listSession   string
		deleteSession string
	)
	list := &cobra.Command{
		Use: "ls", Aliases: []string{"list"}, Short: "List approval rules visible from a session",
		Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listApprovalRules(cmd, provider, listSession, asJSON)
		},
	}
	list.Flags().StringVarP(&listSession, "session", "s", "", "Session whose visible rules should be listed")
	list.Flags().BoolVar(&asJSON, "json", false, "Write approval rules as JSON")
	_ = list.MarkFlagRequired("session")
	command.AddCommand(list)

	var yes bool
	remove := &cobra.Command{
		Use: "delete <rule-id>", Aliases: []string{"rm", "forget"}, Short: "Forget a remembered approval rule",
		Args: cobra.ExactArgs(1), SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("refusing to delete an approval rule without --yes")
			}
			runtime, err := provider.Open(cmd)
			if err != nil {
				return err
			}
			if deleteApprovalRuleErr := runtime.DeleteApprovalRule(cmd.Context(), args[0]); deleteApprovalRuleErr != nil {
				return deleteApprovalRuleErr
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), args[0])
			return err
		},
	}
	remove.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm forgetting the rule")
	remove.Flags().StringVarP(&deleteSession, "session", "s", "", "Session used to complete visible rule IDs")
	remove.ValidArgsFunction = completeApprovalRuleIDs(provider, &deleteSession)
	command.AddCommand(remove)
	return command
}

func completeApprovalRuleIDs(provider runtimeProvider, sessionID *string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 || strings.TrimSpace(*sessionID) == "" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		runtime, err := provider.OpenQuietly(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		rules, err := runtime.ListApprovalRules(cmd.Context(), *sessionID)
		if err != nil || agent.ValidateApprovalRules(rules) != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		items := make([]string, 0, len(rules))
		needle := strings.ToLower(toComplete)
		for _, rule := range rules {
			label := rule.Tool + ":" + rule.Subject
			if needle != "" && !strings.HasPrefix(rule.ID, toComplete) && !strings.Contains(strings.ToLower(label), needle) {
				continue
			}
			items = append(items, rule.ID+"\t"+string(rule.Scope)+" · "+label)
		}
		return items, cobra.ShellCompDirectiveNoFileComp
	}
}

func listApprovalRules(cmd *cobra.Command, provider runtimeProvider, sessionID string, asJSON bool) error {
	runtime, err := provider.Open(cmd)
	if err != nil {
		return err
	}
	rules, err := runtime.ListApprovalRules(cmd.Context(), sessionID)
	if err != nil {
		return err
	}
	if err := agent.ValidateApprovalRules(rules); err != nil {
		return fmt.Errorf("list approval rules: %w", err)
	}
	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Rules []approvalRuleJSON `json:"rules"`
		}{Rules: encodeApprovalRules(rules)})
	}
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, rule := range rules {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", rule.ID, rule.Scope, rule.Decision, rule.Tool, displaySubject(rule.Subject), displaySubject(rule.Dir)); err != nil {
			return err
		}
	}
	return writer.Flush()
}

type approvalRuleJSON struct {
	ID       string `json:"id"`
	Scope    string `json:"scope"`
	Tool     string `json:"tool"`
	Subject  string `json:"subject,omitempty"`
	Dir      string `json:"dir,omitempty"`
	Decision string `json:"decision"`
}

func encodeApprovalRules(rules []agent.ApprovalRule) []approvalRuleJSON {
	encoded := make([]approvalRuleJSON, 0, len(rules))
	for _, rule := range rules {
		encoded = append(encoded, approvalRuleJSON{
			ID: rule.ID, Scope: string(rule.Scope), Tool: rule.Tool, Subject: rule.Subject,
			Dir: rule.Dir, Decision: string(rule.Decision),
		})
	}
	return encoded
}

func displaySubject(value string) string {
	if value == "" {
		return "*"
	}
	return value
}
