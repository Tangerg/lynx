// Package settings owns the CLI's typed, validated user preferences. It knows
// nothing about Viper, files, environment variables, Cobra, or oolong; those
// adapters translate into this product model at their respective boundaries.
package settings

import (
	"errors"
	"fmt"
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

type Settings struct {
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

func Default() Settings {
	return Settings{
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

func (s Settings) Validate() error {
	var problems []error
	if !slices.Contains([]string{"low", "medium", "high", "max", "ultra"}, s.Effort) {
		problems = append(problems, fmt.Errorf("effort %q is not one of low, medium, high, max, ultra", s.Effort))
	}
	if !slices.Contains([]client.AgentMode{client.ModeBuild, client.ModePlan, client.ModeReview}, s.Mode) {
		problems = append(problems, fmt.Errorf("mode %q is not one of build, plan, review", s.Mode))
	}
	if !slices.Contains([]client.PermissionMode{client.PermissionAsk, client.PermissionReadOnly, client.PermissionAutoEdit, client.PermissionFull}, s.Permission) {
		problems = append(problems, fmt.Errorf("permission %q is not one of ask, read-only, auto-edit, full-access", s.Permission))
	}
	if !slices.Contains([]client.RememberScope{client.RememberNone, client.RememberSession, client.RememberProject, client.RememberGlobal}, s.Approval.Remember) {
		problems = append(problems, fmt.Errorf("approval remember scope %q is invalid", s.Approval.Remember))
	}
	if s.UI.TranscriptRetain < 4 || s.UI.TranscriptRetain > 500 {
		problems = append(problems, fmt.Errorf("ui.transcript-retain must be between 4 and 500, got %d", s.UI.TranscriptRetain))
	}
	if s.UI.ReconnectAttempts < 0 || s.UI.ReconnectAttempts > 20 {
		problems = append(problems, fmt.Errorf("ui.reconnect-attempts must be between 0 and 20, got %d", s.UI.ReconnectAttempts))
	}
	pluginDirectories := make(map[string]struct{}, len(s.Plugins.Directories))
	for _, directory := range s.Plugins.Directories {
		directory = strings.TrimSpace(directory)
		if directory == "" {
			problems = append(problems, errors.New("plugins.directories contains an empty path"))
			continue
		}
		if _, duplicate := pluginDirectories[directory]; duplicate {
			problems = append(problems, fmt.Errorf("plugins.directories repeats %q", directory))
		}
		pluginDirectories[directory] = struct{}{}
	}
	knownActions := Default().Keys
	for action, bindings := range s.Keys {
		if _, ok := knownActions[action]; !ok {
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
	return errors.Join(problems...)
}

func (s Settings) RunOptions() client.RunOptions {
	return client.RunOptions{Model: s.Model, Effort: s.Effort, Mode: s.Mode, Permission: s.Permission}
}

func (s Settings) Clone() Settings {
	out := s
	out.Plugins.Directories = slices.Clone(s.Plugins.Directories)
	out.Keys = make(map[string][]string, len(s.Keys))
	for action, bindings := range s.Keys {
		out.Keys[action] = slices.Clone(bindings)
	}
	return out
}
