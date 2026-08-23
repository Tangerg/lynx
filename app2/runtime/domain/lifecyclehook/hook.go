// Package lifecyclehook owns Lyra's user-authored lifecycle policy values.
// Discovery, trust persistence, process execution, and wire projection stay at
// adapter and application boundaries.
package lifecyclehook

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const (
	MaxHooksPerFile   = 128
	MaxHooksPerRun    = 256
	MaxMatcherBytes   = 256
	MaxActionBytes    = 8 << 10
	MaxTimeoutMillis  = 5 * 60 * 1_000
	MaxPromptBytes    = 256 << 10
	MaxArgumentsBytes = 256 << 10
	MaxResultBytes    = 128 << 10
	MaxReasonBytes    = 8 << 10
	MaxContextBytes   = 128 << 10
	MaxRewriteBytes   = 256 << 10
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

func (event Event) contextEvent() bool {
	switch event {
	case PreToolUse, PostToolUse, UserPromptSubmit, SessionStart, PreCompact:
		return true
	default:
		return false
	}
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
	case hasInject && !hook.Event.contextEvent():
		return errors.New("lifecyclehook: inject action has no consumer for this event")
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

type ToolInput struct {
	Name, Arguments, Result, Error string
	ResultTruncated                bool
}

type SubagentInput struct {
	RunID, ParentRunID, Description, Prompt string
	Status                                  SubagentStatus
	Result, Error                           string
	PromptTruncated                         bool
	ResultTruncated                         bool
}

type SubagentStatus string

const (
	SubagentCompleted SubagentStatus = "completed"
	SubagentTimedOut  SubagentStatus = "timed_out"
	SubagentFailed    SubagentStatus = "failed"
	SubagentMaxSteps  SubagentStatus = "max_steps"
	SubagentMaxBudget SubagentStatus = "max_budget"
	SubagentCanceled  SubagentStatus = "canceled"
	SubagentLost      SubagentStatus = "lost"
)

func (status SubagentStatus) Valid() bool {
	switch status {
	case SubagentCompleted, SubagentTimedOut, SubagentFailed,
		SubagentMaxSteps, SubagentMaxBudget, SubagentCanceled, SubagentLost:
		return true
	default:
		return false
	}
}

// Invocation is the stable, provider-neutral lifecycle material exposed to a
// trusted hook command. It contains Lyra identities, never Agent Framework
// process or checkpoint identities.
type Invocation struct {
	Event            Event
	SessionID, RunID string
	Workspace        string
	Prompt           string
	PromptTruncated  bool
	Reason           string
	Tool             *ToolInput
	Subagent         *SubagentInput
}

func (invocation Invocation) Validate() error {
	if !invocation.Event.Valid() {
		return fmt.Errorf("lifecyclehook: invalid invocation event %q", invocation.Event)
	}
	if strings.TrimSpace(invocation.SessionID) == "" ||
		strings.TrimSpace(invocation.RunID) == "" ||
		invocation.SessionID != strings.TrimSpace(invocation.SessionID) ||
		invocation.RunID != strings.TrimSpace(invocation.RunID) {
		return errors.New("lifecyclehook: invocation requires canonical Lyra identities")
	}
	if !filepath.IsAbs(invocation.Workspace) ||
		filepath.Clean(invocation.Workspace) != invocation.Workspace {
		return errors.New("lifecyclehook: invocation workspace must be canonical and absolute")
	}
	if len(invocation.Prompt) > MaxPromptBytes || len(invocation.Reason) > MaxReasonBytes {
		return errors.New("lifecyclehook: invocation text exceeds its boundary")
	}
	reasonEvent := invocation.Event == PreCompact || invocation.Event == Stop ||
		invocation.Event == Notification
	if !reasonEvent && invocation.Reason != "" {
		return errors.New("lifecyclehook: reason is not valid for this event")
	}
	if invocation.Event != UserPromptSubmit &&
		(invocation.Prompt != "" || invocation.PromptTruncated) {
		return errors.New("lifecyclehook: prompt is only valid on user submission")
	}
	if invocation.Event.ToolEvent() != (invocation.Tool != nil) {
		return errors.New("lifecyclehook: tool input must exactly match a tool event")
	}
	subagentEvent := invocation.Event == SubagentStart || invocation.Event == SubagentStop
	if subagentEvent != (invocation.Subagent != nil) {
		return errors.New("lifecyclehook: subagent input must exactly match a subagent event")
	}
	if invocation.Tool != nil {
		if strings.TrimSpace(invocation.Tool.Name) == "" ||
			len(invocation.Tool.Arguments) > MaxArgumentsBytes ||
			len(invocation.Tool.Result) > MaxResultBytes ||
			len(invocation.Tool.Error) > MaxReasonBytes {
			return errors.New("lifecyclehook: invalid bounded tool input")
		}
		if invocation.Event == PreToolUse &&
			(invocation.Tool.Result != "" || invocation.Tool.Error != "" || invocation.Tool.ResultTruncated) {
			return errors.New("lifecyclehook: pre-tool input carries a result")
		}
	}
	if invocation.Subagent != nil {
		if strings.TrimSpace(invocation.Subagent.RunID) == "" ||
			strings.TrimSpace(invocation.Subagent.ParentRunID) == "" ||
			len(invocation.Subagent.Description) > MaxReasonBytes ||
			len(invocation.Subagent.Prompt) > MaxPromptBytes ||
			len(invocation.Subagent.Result) > MaxResultBytes ||
			len(invocation.Subagent.Error) > MaxReasonBytes {
			return errors.New("lifecyclehook: invalid bounded subagent input")
		}
		if invocation.Event == SubagentStart && invocation.Subagent.Status != "" {
			return errors.New("lifecyclehook: subagent start carries terminal status")
		}
		if invocation.Event == SubagentStart &&
			(invocation.Subagent.Result != "" || invocation.Subagent.Error != "" || invocation.Subagent.ResultTruncated) {
			return errors.New("lifecyclehook: subagent start carries terminal material")
		}
		if invocation.Event == SubagentStop && !invocation.Subagent.Status.Valid() {
			return errors.New("lifecyclehook: subagent stop requires terminal status")
		}
	}
	return nil
}

type Verdict string

const (
	VerdictAllow Verdict = "allow"
	VerdictDeny  Verdict = "deny"
	VerdictAsk   Verdict = "ask"
)

func (verdict Verdict) Valid() bool {
	return verdict == VerdictAllow || verdict == VerdictDeny || verdict == VerdictAsk
}

type Context struct {
	Event   Event
	Source  string
	Content string
}

// Decision is the folded lifecycle policy result. Contexts retain provenance;
// consumers choose how to surface them without parsing command output again.
type Decision struct {
	Verdict          Verdict
	Reason           string
	Contexts         []Context
	RewriteArguments string
}

func (decision Decision) Validate(event Event) error {
	verdict := decision.Verdict
	if verdict == "" {
		verdict = VerdictAllow
	}
	if !event.Valid() || !verdict.Valid() {
		return errors.New("lifecyclehook: invalid decision event or verdict")
	}
	if len(decision.Reason) > MaxReasonBytes ||
		len(decision.RewriteArguments) > MaxRewriteBytes {
		return errors.New("lifecyclehook: decision exceeds its boundary")
	}
	if verdict == VerdictAllow && strings.TrimSpace(decision.Reason) != "" {
		return errors.New("lifecyclehook: allow decision carries a reason")
	}
	if verdict == VerdictAsk && event != PreToolUse {
		return errors.New("lifecyclehook: ask is only valid before tool use")
	}
	if decision.RewriteArguments != "" && event != PreToolUse {
		return errors.New("lifecyclehook: argument rewrite is only valid before tool use")
	}
	if verdict == VerdictDeny && decision.RewriteArguments != "" {
		return errors.New("lifecyclehook: deny decision carries an argument rewrite")
	}
	if verdict != VerdictAllow &&
		event != PreToolUse && event != UserPromptSubmit && event != SessionStart && event != PreCompact {
		return errors.New("lifecyclehook: observe-only event returned a control verdict")
	}
	total := 0
	for _, context := range decision.Contexts {
		if context.Event != event || context.Source == "" ||
			!filepath.IsAbs(context.Source) || filepath.Clean(context.Source) != context.Source ||
			strings.TrimSpace(context.Content) == "" || len(context.Content) > MaxActionBytes {
			return errors.New("lifecyclehook: invalid injected context provenance")
		}
		total += len(context.Content)
	}
	if total > MaxContextBytes {
		return errors.New("lifecyclehook: injected context exceeds its boundary")
	}
	if len(decision.Contexts) > 0 && !event.contextEvent() {
		return errors.New("lifecyclehook: injected context has no consumer for this event")
	}
	return nil
}

func (decision Decision) Denied() bool { return decision.Verdict == VerdictDeny }
func (decision Decision) Asks() bool   { return decision.Verdict == VerdictAsk }

func (decision *Decision) Fold(candidate Decision, event Event) error {
	if err := candidate.Validate(event); err != nil {
		return err
	}
	next := *decision
	next.Contexts = slices.Clone(decision.Contexts)
	if next.Verdict == "" {
		next.Verdict = VerdictAllow
	}
	switch candidate.Verdict {
	case VerdictDeny:
		if next.Verdict != VerdictDeny {
			next.Verdict = VerdictDeny
			next.Reason = strings.TrimSpace(candidate.Reason)
		}
		next.RewriteArguments = ""
	case VerdictAsk:
		if next.Verdict == VerdictAllow {
			next.Verdict = VerdictAsk
			next.Reason = strings.TrimSpace(candidate.Reason)
		}
	}
	if next.Verdict != VerdictDeny && next.RewriteArguments == "" {
		next.RewriteArguments = candidate.RewriteArguments
	}
	next.Contexts = append(next.Contexts, candidate.Contexts...)
	if err := next.Validate(event); err != nil {
		return err
	}
	next.Contexts = slices.Clip(next.Contexts)
	*decision = next
	return nil
}
