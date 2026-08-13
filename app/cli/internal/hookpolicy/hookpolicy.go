// Package hookpolicy defines the CLI's auditable projection of runtime
// lifecycle hooks and project trust decisions.
package hookpolicy

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
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

func (event Event) Validate() error {
	switch event {
	case PreToolUse, PostToolUse, UserPromptSubmit, SessionStart, SubagentStart, SubagentStop, PreCompact, Stop, Notification:
		return nil
	default:
		return fmt.Errorf("hook event %q is invalid", event)
	}
}

type Scope string

const (
	Global  Scope = "global"
	Project Scope = "project"
)

func (scope Scope) Validate() error {
	if scope != Global && scope != Project {
		return fmt.Errorf("hook scope %q is invalid", scope)
	}
	return nil
}

type Hook struct {
	Event         Event
	Matcher       string
	Command       string
	Inject        string
	TimeoutMillis int
	Scope         Scope
	Source        string
	Active        bool
}

func (hook Hook) Validate() error {
	if err := hook.Event.Validate(); err != nil {
		return err
	}
	if err := hook.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(hook.Source) == "" {
		return errors.New("hook source is empty")
	}
	hasCommand := strings.TrimSpace(hook.Command) != ""
	hasInject := strings.TrimSpace(hook.Inject) != ""
	if hasCommand == hasInject {
		return errors.New("hook requires exactly one of command or inject")
	}
	if hook.TimeoutMillis < 0 || (hasInject && hook.TimeoutMillis != 0) {
		return errors.New("hook timeout is invalid")
	}
	if hook.Matcher != "" {
		if hook.Event != PreToolUse && hook.Event != PostToolUse {
			return errors.New("hook matcher is only valid for tool events")
		}
		if _, err := path.Match(hook.Matcher, ""); err != nil {
			return fmt.Errorf("hook matcher %q is invalid: %w", hook.Matcher, err)
		}
	}
	return nil
}

type Catalog struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []Hook
}

func (catalog Catalog) Validate() error {
	projectHooks := false
	for index, hook := range catalog.Hooks {
		if err := hook.Validate(); err != nil {
			return fmt.Errorf("hook %d: %w", index+1, err)
		}
		if hook.Scope == Global && !hook.Active {
			return fmt.Errorf("global hook %d is inactive", index+1)
		}
		if hook.Scope == Project {
			projectHooks = true
			if hook.Active != catalog.ProjectTrusted {
				return fmt.Errorf("project hook %d active state disagrees with trust", index+1)
			}
		}
	}
	if (projectHooks || catalog.ProjectTrusted) && strings.TrimSpace(catalog.ProjectRoot) == "" {
		return errors.New("project hooks or trust require a project root")
	}
	return nil
}

// ValidateTrustAcknowledgement proves that an authoritative catalog read after
// SetProjectTrust describes the exact project and trust decision requested.
func (catalog Catalog) ValidateTrustAcknowledgement(projectRoot string, trusted bool) error {
	if err := catalog.Validate(); err != nil {
		return err
	}
	if catalog.ProjectRoot != projectRoot {
		return fmt.Errorf("project hook root is %q, want %q", catalog.ProjectRoot, projectRoot)
	}
	if catalog.ProjectTrusted != trusted {
		return fmt.Errorf("project hook trust is %t, want %t", catalog.ProjectTrusted, trusted)
	}
	return nil
}

type Service interface {
	Catalog(context.Context, string) (Catalog, error)
	SetProjectTrust(context.Context, string, bool) error
}
