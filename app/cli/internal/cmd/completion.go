package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:         "completion <bash|zsh|fish|powershell>",
		Short:       "Generate a shell completion script",
		Args:        cobra.ExactArgs(1),
		ValidArgs:   []string{"bash", "zsh", "fish", "powershell"},
		Annotations: map[string]string{configIndependentAnnotation: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
}
