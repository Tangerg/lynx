// Package lifecyclehook owns Lyra's user-authored lifecycle policy values.
// Discovery, trust persistence, process execution, and wire projection stay at
// adapter and application boundaries.
package lifecyclehook

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

const (
	MaxHooksPerFile   = 128
	MaxHooksPerRun    = 256
	MaxMatcherBytes   = 256
	MaxActionBytes    = 8 << 10
	MaxTimeoutMillis  = 5 * 60 * 1_000
)

type Event string

const (
	PreToolUse       Event = "PreToolUse"
	PostToolUse      Event = "PostToolUse"
	UserPromptSubmit Event = "UserPromptSubmit"
	SessionStart     Event = "SessionStart"
	SubagentStart    Event = "SubagentStart"
	SubagentStop     Event = "SubagentStop"
	PreCompact       Event = "PreCompact"
	Stop             Event = "Stop"
	Notification     Event = "Notification"
)

func (event Event) Valid() bool {
	switch event {
	case PreToolUse, PostToolUse, UserPromptSubmit, SessionStart,
		SubagentStart, SubagentStop, PreCompact, Stop, Notification:
		return true
	default:
		return false
	}
}

func (event Event) ToolEvent() bool {
	return event == PreToolUse || event == PostToolUse
}

type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
)

func (scope Scope) Valid() bool {
	return scope == ScopeGlobal || scope == ScopeProject
}

type Hook struct {
	Event         Event
	Matcher       string
	Command       string
	Inject        string
	TimeoutMillis int
	Scope         Scope
	Source        string
}

type Target struct {
	ProjectRoot string
	Workspace   string
}

func (target Target) Validate() error {
	if !filepath.IsAbs(target.ProjectRoot) ||
		filepath.Clean(target.ProjectRoot) != target.ProjectRoot ||
		!filepath.IsAbs(target.Workspace) ||
		filepath.Clean(target.Workspace) != target.Workspace {
		return errors.New("lifecyclehook: target paths must be canonical and absolute")
	}
	relative, err := filepath.Rel(target.ProjectRoot, target.Workspace)
	if err != nil || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("lifecyclehook: workspace is outside its project root")
	}
	return nil
}

func (hook Hook) Validate() error {
	hasCommand := strings.TrimSpace(hook.Command) != ""
	hasInject := strings.TrimSpace(hook.Inject) != ""
	switch {
	case !hook.Event.Valid():
		return fmt.Errorf("lifecyclehook: invalid event %q", hook.Event)
	case !hook.Scope.Valid():
		return fmt.Errorf("lifecyclehook: invalid scope %q", hook.Scope)
	case !filepath.IsAbs(hook.Source) || filepath.Clean(hook.Source) != hook.Source:
		return errors.New("lifecyclehook: source must be canonical and absolute")
	case hasCommand == hasInject:
		return errors.New("lifecyclehook: exactly one action is required")
	case len(hook.Command) > MaxActionBytes || len(hook.Inject) > MaxActionBytes:
		return fmt.Errorf("lifecyclehook: action exceeds %d bytes", MaxActionBytes)
	case len(hook.Matcher) > MaxMatcherBytes:
		return fmt.Errorf("lifecyclehook: matcher exceeds %d bytes", MaxMatcherBytes)
	case hook.Matcher != "" && !hook.Event.ToolEvent():
		return errors.New("lifecyclehook: matcher is only valid for tool events")
	case hook.TimeoutMillis < 0 || hook.TimeoutMillis > MaxTimeoutMillis:
		return fmt.Errorf(
			"lifecyclehook: timeoutMillis must be between 0 and %d",
			MaxTimeoutMillis,
		)
	case hasInject && hook.TimeoutMillis != 0:
		return errors.New("lifecyclehook: inject action forbids a timeout")
	}
	if hook.Matcher != "" {
		if _, err := path.Match(hook.Matcher, ""); err != nil {
			return fmt.Errorf("lifecyclehook: invalid matcher %q: %w", hook.Matcher, err)
		}
	}
	return nil
}

func (hook Hook) Matches(event Event, toolName string) bool {
	if hook.Event != event {
		return false
	}
	if !event.ToolEvent() || hook.Matcher == "" {
		return true
	}
	matched, err := path.Match(hook.Matcher, toolName)
	return err == nil && matched
}
