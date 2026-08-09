package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

func configure(v *viper.Viper, root *cobra.Command) {
	defaults := settings.Default()
	setDefaults(v, defaults)
	v.SetEnvPrefix("LYRA")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	flags := root.PersistentFlags()
	flags.String("config", "", "Configuration file (default: ./.lyra.yaml or the user config directory)")
	flags.String("model", defaults.Model, "Model used for new runs")
	flags.String("effort", defaults.Effort, "Reasoning effort: low, medium, high, max, or ultra")
	flags.String("mode", string(defaults.Mode), "Agent mode: build, plan, or review")
	flags.String("permission", string(defaults.Permission), "Permission mode: ask, read-only, auto-edit, or full-access")
	flags.Bool("mouse", defaults.UI.Mouse, "Enable mouse input in the terminal UI")
	flags.Bool("notifications", defaults.UI.Notifications, "Enable terminal completion notifications")
	flags.Bool("tool-details", defaults.UI.ToolDetails, "Expand tool output and diffs by default")
	flags.Int("transcript-retain", defaults.UI.TranscriptRetain, "Finished blocks retained in the live terminal viewport")
	flags.Int("reconnect-attempts", defaults.UI.ReconnectAttempts, "Times to reconnect a dropped run subscription")
	flags.StringSlice("plugin-dir", defaults.Plugins.Directories, "Directory containing sideloaded plugins (repeatable)")
}

func setDefaults(v *viper.Viper, defaults settings.Settings) {
	v.SetDefault("model", defaults.Model)
	v.SetDefault("effort", defaults.Effort)
	v.SetDefault("mode", defaults.Mode)
	v.SetDefault("permission", defaults.Permission)
	v.SetDefault("approval.remember", defaults.Approval.Remember)
	v.SetDefault("ui.mouse", defaults.UI.Mouse)
	v.SetDefault("ui.notifications", defaults.UI.Notifications)
	v.SetDefault("ui.tool-details", defaults.UI.ToolDetails)
	v.SetDefault("ui.transcript-retain", defaults.UI.TranscriptRetain)
	v.SetDefault("ui.reconnect-attempts", defaults.UI.ReconnectAttempts)
	v.SetDefault("plugins.directories", defaults.Plugins.Directories)
	for action, bindings := range defaults.Keys {
		v.SetDefault("keys."+action, bindings)
	}
}

func loadConfig(v *viper.Viper, cmd *cobra.Command) error {
	path, err := cmd.Flags().GetString("config")
	if err != nil {
		return err
	}
	if path != "" {
		v.SetConfigFile(path)
	} else {
		projectConfig := ".lyra.yaml"
		_, projectErr := os.Stat(projectConfig)
		if projectErr == nil {
			v.SetConfigFile(projectConfig)
		} else if !errors.Is(projectErr, os.ErrNotExist) {
			return fmt.Errorf("inspect project configuration: %w", projectErr)
		}
		if errors.Is(projectErr, os.ErrNotExist) {
			configDir, err := os.UserConfigDir()
			if err != nil {
				return fmt.Errorf("resolve user config directory: %w", err)
			}
			v.SetConfigName("config")
			v.SetConfigType("yaml")
			v.AddConfigPath(filepath.Join(configDir, "lyra"))
		}
	}
	if err := v.ReadInConfig(); err != nil {
		_, notFound := errors.AsType[viper.ConfigFileNotFoundError](err)
		if path != "" || !notFound {
			return fmt.Errorf("read configuration: %w", err)
		}
	}
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("bind command flags: %w", err)
	}
	for key, flag := range map[string]string{
		"ui.mouse": "mouse", "ui.notifications": "notifications",
		"ui.tool-details":      "tool-details",
		"ui.transcript-retain": "transcript-retain", "ui.reconnect-attempts": "reconnect-attempts",
		"plugins.directories": "plugin-dir",
	} {
		if found := cmd.Flags().Lookup(flag); found != nil {
			if err := v.BindPFlag(key, found); err != nil {
				return fmt.Errorf("bind --%s: %w", flag, err)
			}
		}
	}
	_, err = readSettings(v)
	return err
}

func readSettings(v *viper.Viper) (settings.Settings, error) {
	var value settings.Settings
	if err := v.Unmarshal(&value); err != nil {
		return settings.Settings{}, fmt.Errorf("decode configuration: %w", err)
	}
	if len(value.Plugins.Directories) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return settings.Settings{}, fmt.Errorf("resolve home directory for plugins: %w", err)
		}
		value.Plugins.Directories = []string{filepath.Join(home, ".lyra", "plugins")}
	}
	if err := value.Validate(); err != nil {
		return settings.Settings{}, fmt.Errorf("validate configuration: %w", err)
	}
	return value.Clone(), nil
}

func newConfigCommand(v *viper.Viper) *cobra.Command {
	config := &cobra.Command{Use: "config", Short: "Inspect the effective configuration", Args: cobra.NoArgs}
	config.AddCommand(&cobra.Command{
		Use:          "show",
		Short:        "Print the merged, validated configuration as JSON",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			value, err := readSettings(v)
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(value)
		},
	})
	config.AddCommand(&cobra.Command{
		Use:          "path",
		Short:        "Print the configuration file that was loaded",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			used := v.ConfigFileUsed()
			if used == "" {
				used = "(defaults, environment, and flags only)"
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), used)
			return err
		},
	})
	return config
}
