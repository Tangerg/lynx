package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

func newRuntimeCommand(provider runtimeProvider) *cobra.Command {
	command := &cobra.Command{
		Use:   "runtime",
		Short: "Inspect the connected runtime",
	}
	command.AddCommand(newRuntimeInfoCommand(provider))
	return command
}

func newRuntimeInfoCommand(provider runtimeProvider) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "info",
		Short: "Show discovery identity, capabilities, and hard limits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			services, err := provider.OpenServices(cmd)
			if err != nil {
				return err
			}
			if services.RuntimeProfile == nil {
				return errors.New("runtime discovery profile is unavailable")
			}
			profile := services.RuntimeProfile.Clone()
			if err := profile.Validate(); err != nil {
				return err
			}
			if asJSON {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetEscapeHTML(false)
				return encoder.Encode(profile)
			}
			return writeRuntimeProfile(cmd.OutOrStdout(), profile)
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "Write the complete profile as JSON")
	return command
}

func writeRuntimeProfile(output io.Writer, profile runtimeprofile.Profile) error {
	writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	rows := [][2]string{
		{"runtime", profile.Server.Name + " " + profile.Server.Version},
		{"protocol", profile.Protocol.Current + " (minimum " + profile.Protocol.MinSupported + ")"},
		{"default workspace", profile.Server.DefaultWorkspace},
		{"home", profile.Server.Home},
		{"run events", strings.Join(profile.RunEvents, ", ")},
		{"runtime topics", strings.Join(profile.RuntimeTopics, ", ")},
		{"streaming methods", strings.Join(profile.StreamingMethods, ", ")},
	}
	for _, snapshot := range profile.StateSnapshots {
		rows = append(rows, [2]string{"state " + snapshot.Key, strings.Join([]string{snapshot.RecoveryMethod, snapshot.Scope, snapshot.Writer}, " · ")})
	}
	for _, name := range slices.Sorted(maps.Keys(profile.Features)) {
		feature := profile.Features[name]
		flags := []string{string(feature.Stability)}
		if feature.Enabled {
			flags = append(flags, "enabled")
		} else {
			flags = append(flags, "disabled")
		}
		if feature.ClientOptIn {
			if feature.ClientRequested {
				flags = append(flags, "client opt-in requested")
			} else {
				flags = append(flags, "client opt-in declined")
			}
		}
		if feature.RequiredByRunProtocol {
			flags = append(flags, "run protocol")
		}
		if feature.Available() {
			flags = append(flags, "available")
		}
		rows = append(rows, [2]string{"feature " + string(name), strings.Join(flags, " · ")})
	}
	limits := profile.Limits
	rows = append(rows,
		[2]string{"max concurrent runs", formatOptionalRuntimeLimit(limits.MaxConcurrentRuns)},
		[2]string{"idempotency retention", fmt.Sprintf("%d seconds", limits.IdempotencyRetentionSeconds)},
		[2]string{"run replay", fmt.Sprintf("%d events · %d bytes · %s", limits.RunReplay.MaxEvents, limits.RunReplay.MaxBytes, limits.RunReplay.Scope)},
		[2]string{"MCP auth retention", fmt.Sprintf("%d seconds", limits.MCPAuthorizationRetentionSeconds)},
		[2]string{"runtime subscription", fmt.Sprintf("%d topics · %d watches", limits.RuntimeSubscription.MaxTopics, limits.RuntimeSubscription.MaxWatches)},
	)
	for _, row := range rows {
		if _, err := fmt.Fprintf(writer, "%s\t%s\n", row[0], row[1]); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func formatOptionalRuntimeLimit(value int) string {
	if value == 0 {
		return "runtime default"
	}
	return fmt.Sprintf("%d", value)
}
