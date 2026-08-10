package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func newApprovalsCommand(provider runtimeProvider) *cobra.Command {
	command := &cobra.Command{Use: "approvals", Short: "Inspect remembered approval rules", Args: cobra.NoArgs}
	var asJSON bool
	list := &cobra.Command{
		Use:          "ls",
		Aliases:      []string{"list"},
		Short:        "List remembered approval rules",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listApprovalRules(cmd, provider, asJSON)
		},
	}
	list.Flags().BoolVar(&asJSON, "json", false, "Write approval rules as JSON")
	command.AddCommand(list)
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
			runtime, err := provider.Open(cmd)
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
	remove.ValidArgsFunction = completeApprovalRuleIDs(provider)
	command.AddCommand(remove)
	return command
}

func completeApprovalRuleIDs(provider runtimeProvider) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		runtime, err := provider.OpenForCompletion(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		rules, err := runtime.ListApprovalRules(cmd.Context())
		if err != nil || client.ValidateApprovalRules(rules) != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		items := make([]string, 0, len(rules))
		needle := strings.ToLower(toComplete)
		for _, rule := range rules {
			if needle != "" && !strings.HasPrefix(rule.ID, toComplete) && !strings.Contains(strings.ToLower(rule.Rule), needle) {
				continue
			}
			items = append(items, rule.ID+"\t"+string(rule.Scope)+" · "+rule.Rule)
		}
		return items, cobra.ShellCompDirectiveNoFileComp
	}
}

func listApprovalRules(cmd *cobra.Command, provider runtimeProvider, asJSON bool) error {
	runtime, err := provider.Open(cmd)
	if err != nil {
		return err
	}
	rules, err := runtime.ListApprovalRules(cmd.Context())
	if err != nil {
		return err
	}
	if err := client.ValidateApprovalRules(rules); err != nil {
		return fmt.Errorf("list approval rules: %w", err)
	}
	if asJSON {
		return writeApprovalRulesJSON(cmd, rules)
	}
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, rule := range rules {
		if err := writeApprovalRule(writer, rule); err != nil {
			return err
		}
	}
	return writer.Flush()
}

type approvalRulesJSON struct {
	Rules []approvalRuleJSON `json:"rules"`
}

type approvalRuleJSON struct {
	ID        string `json:"id"`
	Scope     string `json:"scope"`
	Decision  string `json:"decision"`
	SessionID string `json:"sessionId,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Rule      string `json:"rule"`
}

func writeApprovalRulesJSON(cmd *cobra.Command, rules []client.ApprovalRule) error {
	output := approvalRulesJSON{Rules: make([]approvalRuleJSON, 0, len(rules))}
	for _, rule := range rules {
		output.Rules = append(output.Rules, approvalRuleJSON{
			ID: rule.ID, Scope: string(rule.Scope), Decision: string(rule.Decision),
			SessionID: rule.SessionID, Workspace: rule.Workspace, Rule: rule.Rule,
		})
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
}

func writeApprovalRule(writer *tabwriter.Writer, rule client.ApprovalRule) error {
	target := rule.Workspace
	if target == "" {
		target = rule.SessionID
	}
	if target == "" {
		target = "*"
	}
	_, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", rule.ID, rule.Scope, rule.Decision, target, rule.Rule)
	return err
}
