package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

type flagBinding struct {
	key  string
	flag string
}

var settingFlagBindings = [...]flagBinding{
	{key: "model", flag: "model"},
	{key: "effort", flag: "effort"},
	{key: "mode", flag: "mode"},
	{key: "permission", flag: "permission"},
	{key: "ui.mouse", flag: "mouse"},
	{key: "ui.notifications", flag: "notifications"},
	{key: "ui.tool-details", flag: "tool-details"},
	{key: "ui.transcript-retain", flag: "transcript-retain"},
	{key: "ui.reconnect-attempts", flag: "reconnect-attempts"},
	{key: "plugins.directories", flag: "plugin-dir"},
}

func configureRoot(v *viper.Viper, root *cobra.Command) {
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

func setDefaults(v *viper.Viper, defaults settings.Config) {
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
	for _, action := range slices.Sorted(maps.Keys(defaults.Keys)) {
		bindings := defaults.Keys[action]
		v.SetDefault("keys."+action, bindings)
	}
}

func loadConfig(v *viper.Viper, cmd *cobra.Command) error {
	path, err := cmd.Flags().GetString("config")
	if err != nil {
		return err
	}
	if err := selectConfigSource(v, path); err != nil {
		return err
	}
	if err := v.ReadInConfig(); err != nil {
		_, notFound := errors.AsType[viper.ConfigFileNotFoundError](err)
		if path != "" || !notFound {
			return fmt.Errorf("read configuration: %w", err)
		}
	}
	if err := bindSettingFlags(v, cmd); err != nil {
		return err
	}
	_, err = readSettings(v)
	return err
}

func selectConfigSource(v *viper.Viper, explicitPath string) error {
	if explicitPath != "" {
		v.SetConfigFile(explicitPath)
		return nil
	}
	const projectConfig = ".lyra.yaml"
	_, err := os.Stat(projectConfig)
	switch {
	case err == nil:
		v.SetConfigFile(projectConfig)
		return nil
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect project configuration: %w", err)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve user config directory: %w", err)
	}
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(filepath.Join(configDir, "lyra"))
	return nil
}

func bindSettingFlags(v *viper.Viper, cmd *cobra.Command) error {
	for _, binding := range settingFlagBindings {
		flag := cmd.Flags().Lookup(binding.flag)
		if flag == nil {
			continue
		}
		if err := v.BindPFlag(binding.key, flag); err != nil {
			return fmt.Errorf("bind --%s: %w", binding.flag, err)
		}
	}
	return nil
}

func readSettings(v *viper.Viper) (settings.Config, error) {
	var config settings.Config
	if err := v.UnmarshalExact(&config); err != nil {
		return settings.Config{}, fmt.Errorf("decode configuration: %w", err)
	}
	if len(config.Plugins.Directories) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return settings.Config{}, fmt.Errorf("resolve home directory for plugins: %w", err)
		}
		config.Plugins.Directories = []string{filepath.Join(home, ".lyra", "plugins")}
	}
	if err := config.Validate(); err != nil {
		return settings.Config{}, fmt.Errorf("validate configuration: %w", err)
	}
	return config.Clone(), nil
}

func newConfigCommand(v *viper.Viper) *cobra.Command {
	config := &cobra.Command{Use: "config", Short: "Inspect the effective configuration", Args: cobra.NoArgs}
	config.AddCommand(&cobra.Command{
		Use:          "show",
		Short:        "Print the merged, validated configuration as JSON",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			config, err := readSettings(v)
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(config)
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
