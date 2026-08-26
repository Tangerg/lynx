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

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const (
	DefaultProvider = "deepseek"
	DefaultModel    = "deepseek-v4-flash"

	ActionSend            = "send"
	ActionNewline         = "newline"
	ActionCancelRun       = "cancel-run"
	ActionQuit            = "quit"
	ActionCommandPalette  = "command-palette"
	ActionShortcuts       = "shortcuts"
	ActionSessions        = "sessions"
	ActionSearch          = "search"
	ActionManageQueue     = "manage-queue"
	ActionChooseModel     = "choose-model"
	ActionToggleDetails   = "toggle-details"
	ActionHistoryPrevious = "history-previous"
	ActionHistoryNext     = "history-next"
	ActionNextMatch       = "next-match"
	ActionPreviousMatch   = "previous-match"
	ActionScrollPageUp    = "scroll-page-up"
	ActionScrollPageDown  = "scroll-page-down"
	ActionScrollTop       = "scroll-top"
	ActionScrollBottom    = "scroll-bottom"
	ActionExternalEditor  = "external-editor"
)

type Config struct {
	Provider string              `json:"provider" mapstructure:"provider"`
	Model    string              `json:"model"    mapstructure:"model"`
	Run      Run                 `json:"run"      mapstructure:"run"`
	Approval Approval            `json:"approval" mapstructure:"approval"`
	UI       UI                  `json:"ui"       mapstructure:"ui"`
	Plugins  Plugins             `json:"plugins"  mapstructure:"plugins"`
	Keys     map[string][]string `json:"keys"     mapstructure:"keys"`
}

type Run struct {
	MaxTotalTokens int64   `json:"maxTotalTokens" mapstructure:"max-total-tokens"`
	MaxSteps       int     `json:"maxSteps"       mapstructure:"max-steps"`
	MaxBudgetUSD   float64 `json:"maxBudgetUsd"   mapstructure:"max-budget-usd"`
}

type Approval struct {
	Remember RememberPreference `json:"remember" mapstructure:"remember"`
}

// RememberPreference is the explicit configuration vocabulary. It deliberately
// differs from agent.RememberScope, whose zero value means the protocol answer
// omits a remember directive.
type RememberPreference string

const (
	RememberNone    RememberPreference = "none"
	RememberSession RememberPreference = "session"
	RememberProject RememberPreference = "project"
	RememberGlobal  RememberPreference = "global"
)

func (r RememberPreference) Scope() agent.RememberScope {
	switch r {
	case RememberSession:
		return agent.RememberSession
	case RememberProject:
		return agent.RememberProject
	case RememberGlobal:
		return agent.RememberGlobal
	default:
		return agent.RememberNone
	}
}

type UI struct {
	Mouse             bool `json:"mouse"             mapstructure:"mouse"`
	Notifications     bool `json:"notifications"     mapstructure:"notifications"`
	ToolDetails       bool `json:"toolDetails"       mapstructure:"tool-details"`
	TranscriptRetain  int  `json:"transcriptRetain"  mapstructure:"transcript-retain"`
	ReconnectAttempts int  `json:"reconnectAttempts" mapstructure:"reconnect-attempts"`
}

type Plugins struct {
	Directories []string `json:"directories" mapstructure:"directories"`
}

func Default() Config {
	return Config{
		Provider: DefaultProvider,
		Model:    DefaultModel,
		Approval: Approval{Remember: RememberNone},
		UI:       UI{Mouse: true, Notifications: true, ToolDetails: false, TranscriptRetain: 24, ReconnectAttempts: 4},
		Keys: map[string][]string{
			ActionSend:            {"enter"},
			ActionNewline:         {"shift+enter", "alt+enter"},
			ActionCancelRun:       {"ctrl+c"},
			ActionQuit:            {"ctrl+q", "ctrl+d"},
			ActionCommandPalette:  {"ctrl+p"},
			ActionShortcuts:       {"ctrl+x"},
			ActionSessions:        {"ctrl+r"},
			ActionSearch:          {"ctrl+f"},
			ActionManageQueue:     {"ctrl+;", "ctrl+g"},
			ActionChooseModel:     {"shift+tab"},
			ActionToggleDetails:   {"ctrl+o"},
			ActionHistoryPrevious: {"alt+up"},
			ActionHistoryNext:     {"alt+down"},
			ActionNextMatch:       {"f3"},
			ActionPreviousMatch:   {"shift+f3"},
			ActionScrollPageUp:    {"pageup"},
			ActionScrollPageDown:  {"pagedown"},
			ActionScrollTop:       {"ctrl+home"},
			ActionScrollBottom:    {"ctrl+end"},
			ActionExternalEditor:  {"ctrl+e"},
		},
	}
}

func (c Config) Validate() error {
	var problems []error
	if err := c.RunOptions().Validate(); err != nil {
		problems = append(problems, err)
	}
	problems = append(problems, validateApproval(c.Approval)...)
	problems = append(problems, validateUI(c.UI)...)
	problems = append(problems, validatePluginDirectories(c.Plugins.Directories)...)
	problems = append(problems, validateKeys(c.Keys)...)
	return errors.Join(problems...)
}

func validateApproval(approval Approval) []error {
	if !slices.Contains([]RememberPreference{RememberNone, RememberSession, RememberProject, RememberGlobal}, approval.Remember) {
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
	for _, action := range slices.Sorted(maps.Keys(known)) {
		if _, ok := keys[action]; !ok {
			problems = append(problems, fmt.Errorf("keys.%s is missing", action))
		}
	}
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

func (c Config) RunOptions() agent.RunOptions {
	return agent.RunOptions{
		Provider: c.Provider,
		Model:    c.Model,
		Limits: agent.RunLimits{
			MaxTotalTokens: c.Run.MaxTotalTokens,
			MaxSteps:       c.Run.MaxSteps,
			MaxBudgetUSD:   c.Run.MaxBudgetUSD,
		},
	}
}

func (c Config) Clone() Config {
	out := c
	out.Plugins.Directories = slices.Clone(c.Plugins.Directories)
	out.Keys = make(map[string][]string, len(c.Keys))
	for action, bindings := range c.Keys {
		out.Keys[action] = slices.Clone(bindings)
	}
	return out
}
