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

func (e Event) Validate() error {
	switch e {
	case PreToolUse, PostToolUse, UserPromptSubmit, SessionStart, SubagentStart, SubagentStop, PreCompact, Stop, Notification:
		return nil
	default:
		return fmt.Errorf("hook event %q is invalid", e)
	}
}

type Scope string

const (
	Global  Scope = "global"
	Project Scope = "project"
)

func (s Scope) Validate() error {
	if s != Global && s != Project {
		return fmt.Errorf("hook scope %q is invalid", s)
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

func (h Hook) Validate() error {
	if err := h.Event.Validate(); err != nil {
		return err
	}
	if err := h.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(h.Source) == "" {
		return errors.New("hook source is empty")
	}
	hasCommand := strings.TrimSpace(h.Command) != ""
	hasInject := strings.TrimSpace(h.Inject) != ""
	if hasCommand == hasInject {
		return errors.New("hook requires exactly one of command or inject")
	}
	if h.TimeoutMillis < 0 || (hasInject && h.TimeoutMillis != 0) {
		return errors.New("hook timeout is invalid")
	}
	if h.Matcher != "" {
		if h.Event != PreToolUse && h.Event != PostToolUse {
			return errors.New("hook matcher is only valid for tool events")
		}
		if _, err := path.Match(h.Matcher, ""); err != nil {
			return fmt.Errorf("hook matcher %q is invalid: %w", h.Matcher, err)
		}
	}
	return nil
}

type Catalog struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []Hook
}

func (c Catalog) Validate() error {
	projectHooks := false
	for index, hook := range c.Hooks {
		if err := hook.Validate(); err != nil {
			return fmt.Errorf("hook %d: %w", index+1, err)
		}
		if hook.Scope == Global && !hook.Active {
			return fmt.Errorf("global hook %d is inactive", index+1)
		}
		if hook.Scope == Project {
			projectHooks = true
			if hook.Active != c.ProjectTrusted {
				return fmt.Errorf("project hook %d active state disagrees with trust", index+1)
			}
		}
	}
	if (projectHooks || c.ProjectTrusted) && strings.TrimSpace(c.ProjectRoot) == "" {
		return errors.New("project hooks or trust require a project root")
	}
	return nil
}

// ValidateTrustAcknowledgement proves that an authoritative catalog read after
// SetProjectTrust describes the exact project and trust decision requested.
func (c Catalog) ValidateTrustAcknowledgement(projectRoot string, trusted bool) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.ProjectRoot != projectRoot {
		return fmt.Errorf("project hook root is %q, want %q", c.ProjectRoot, projectRoot)
	}
	if c.ProjectTrusted != trusted {
		return fmt.Errorf("project hook trust is %t, want %t", c.ProjectTrusted, trusted)
	}
	return nil
}

type Service interface {
	Catalog(context.Context, string) (Catalog, error)
	SetProjectTrust(context.Context, string, bool) error
}
