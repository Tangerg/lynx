// Package settings owns the CLI's typed, validated user preferences. It knows
// nothing about Viper, files, environment variables, Cobra, or oolong; those
// adapters translate into this product model at their respective boundaries.
package settings

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

const (
	ActionSend            = "send"
	ActionCancelRun       = "cancel-run"
	ActionQuit            = "quit"
	ActionCommandPalette  = "command-palette"
	ActionSessions        = "sessions"
	ActionSearch          = "search"
	ActionCycleMode       = "cycle-mode"
	ActionToggleDetails   = "toggle-details"
	ActionHistoryPrevious = "history-previous"
	ActionHistoryNext     = "history-next"
	ActionNextMatch       = "next-match"
	ActionPreviousMatch   = "previous-match"
)

type Config struct {
	Model      string                `json:"model" mapstructure:"model"`
	Effort     string                `json:"effort" mapstructure:"effort"`
	Mode       client.AgentMode      `json:"mode" mapstructure:"mode"`
	Permission client.PermissionMode `json:"permission" mapstructure:"permission"`
	Approval   Approval              `json:"approval" mapstructure:"approval"`
	UI         UI                    `json:"ui" mapstructure:"ui"`
	Plugins    Plugins               `json:"plugins" mapstructure:"plugins"`
	Keys       map[string][]string   `json:"keys" mapstructure:"keys"`
}

type Approval struct {
	Remember client.RememberScope `json:"remember" mapstructure:"remember"`
}

type UI struct {
	Mouse             bool `json:"mouse" mapstructure:"mouse"`
	Notifications     bool `json:"notifications" mapstructure:"notifications"`
	ToolDetails       bool `json:"toolDetails" mapstructure:"tool-details"`
	TranscriptRetain  int  `json:"transcriptRetain" mapstructure:"transcript-retain"`
	ReconnectAttempts int  `json:"reconnectAttempts" mapstructure:"reconnect-attempts"`
}

type Plugins struct {
	Directories []string `json:"directories" mapstructure:"directories"`
}

func Default() Config {
	return Config{
		Effort: "medium", Mode: client.ModeBuild, Permission: client.PermissionAsk,
		Approval: Approval{Remember: client.RememberNone},
		UI:       UI{Mouse: true, Notifications: true, ToolDetails: false, TranscriptRetain: 24, ReconnectAttempts: 4},
		Keys: map[string][]string{
			ActionSend:            {"enter"},
			ActionCancelRun:       {"ctrl+x"},
			ActionQuit:            {"ctrl+c"},
			ActionCommandPalette:  {"ctrl+p"},
			ActionSessions:        {"ctrl+r"},
			ActionSearch:          {"ctrl+f"},
			ActionCycleMode:       {"shift+tab"},
			ActionToggleDetails:   {"ctrl+o"},
			ActionHistoryPrevious: {"alt+up"},
			ActionHistoryNext:     {"alt+down"},
			ActionNextMatch:       {"f3"},
			ActionPreviousMatch:   {"shift+f3"},
		},
	}
}

func (s Config) Validate() error {
	var problems []error
	if err := s.RunOptions().Validate(); err != nil {
		problems = append(problems, err)
	}
	problems = append(problems, validateApproval(s.Approval)...)
	problems = append(problems, validateUI(s.UI)...)
	problems = append(problems, validatePluginDirectories(s.Plugins.Directories)...)
	problems = append(problems, validateKeys(s.Keys)...)
	return errors.Join(problems...)
}

func validateApproval(approval Approval) []error {
	if !slices.Contains([]client.RememberScope{client.RememberNone, client.RememberSession, client.RememberProject, client.RememberGlobal}, approval.Remember) {
		return []error{fmt.Errorf("approval remember scope %q is invalid", approval.Remember)}
	}
	return nil
}

func validateUI(ui UI) []error {
	var problems []error
	if ui.TranscriptRetain < 4 || ui.TranscriptRetain > 500 {
		problems = append(problems, fmt.Errorf("ui.transcript-retain must be between 4 and 500, got %d", ui.TranscriptRetain))
	}
	if ui.ReconnectAttempts < 0 || ui.ReconnectAttempts > 20 {
		problems = append(problems, fmt.Errorf("ui.reconnect-attempts must be between 0 and 20, got %d", ui.ReconnectAttempts))
	}
	return problems
}

func validatePluginDirectories(directories []string) []error {
	var problems []error
	seen := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		directory = strings.TrimSpace(directory)
		if directory == "" {
			problems = append(problems, errors.New("plugins.directories contains an empty path"))
			continue
		}
		if _, duplicate := seen[directory]; duplicate {
			problems = append(problems, fmt.Errorf("plugins.directories repeats %q", directory))
		}
		seen[directory] = struct{}{}
	}
	return problems
}

func validateKeys(keys map[string][]string) []error {
	known := Default().Keys
	var problems []error
	for _, action := range slices.Sorted(maps.Keys(keys)) {
		bindings := keys[action]
		if _, ok := known[action]; !ok {
			problems = append(problems, fmt.Errorf("keys.%s is not a known action", action))
		}
		if len(bindings) == 0 {
			problems = append(problems, fmt.Errorf("keys.%s has no bindings", action))
		}
		for _, binding := range bindings {
			if strings.TrimSpace(binding) == "" {
				problems = append(problems, fmt.Errorf("keys.%s contains an empty binding", action))
			}
		}
	}
	return problems
}

func (s Config) RunOptions() client.RunOptions {
	return client.RunOptions{Model: s.Model, Effort: s.Effort, Mode: s.Mode, Permission: s.Permission}
}

func (s Config) Clone() Config {
	out := s
	out.Plugins.Directories = slices.Clone(s.Plugins.Directories)
	out.Keys = make(map[string][]string, len(s.Keys))
	for action, bindings := range s.Keys {
		out.Keys[action] = slices.Clone(bindings)
	}
	return out
}
