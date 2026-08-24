// Package hooks defines user-configurable lifecycle hook values, event matching,
// and decision combination. A hook either contributes declarative context or
// names a command action; configuration discovery, trust persistence, and action
// execution remain outside this pure domain package.
package hooks

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	// MaxConfigurationFileBytes bounds one complete authored hooks.json file
	// before JSON decoding or management projection.
	MaxConfigurationFileBytes int64 = 256 << 10
	// MaxHooksPerFile bounds one authored policy document.
	MaxHooksPerFile = 128
	// MaxHooksPerCascade bounds the complete global + project policy resolved
	// for one workspace.
	MaxHooksPerCascade = 256
	// MaxMatcherBytes bounds one tool-name glob.
	MaxMatcherBytes = 256
	// MaxActionBytes bounds one shell command or declarative injection.
	MaxActionBytes = 8 << 10
	// MaxPromptBytes bounds user and sub-agent prompt material presented to one
	// command hook. Longer prompt material is projected as a marked prefix.
	MaxPromptBytes = 256 << 10
	// MaxArgumentsBytes is the lossless ceiling for canonical Tool arguments.
	// Arguments are policy input, so an oversized object is rejected rather than
	// truncated into a different action.
	MaxArgumentsBytes = 256 << 10
	// MaxResultBytes bounds Tool and sub-agent result material presented to one
	// command hook. The original result remains owned by its normal consumer.
	MaxResultBytes = 128 << 10
	// MaxReasonBytes bounds failure, terminal, and descriptive material exposed
	// to one command hook.
	MaxReasonBytes = 8 << 10
	// MaxTimeoutMillis prevents authored policy from replacing the Runtime's
	// bounded command lifecycle with an arbitrarily long wait.
	MaxTimeoutMillis = 5 * 60 * 1_000
)

// Event is a lifecycle point a hook can fire on.
type Event string

const (
	// PreToolUse fires before a tool runs — a hook may deny, force an approval
	// prompt, or rewrite the tool's arguments. Matched by tool name.
	PreToolUse Event = "PreToolUse"
	// PostToolUse fires after a tool produced its result — a hook may inject
	// context for the model (e.g. lint output). Matched by tool name.
	PostToolUse Event = "PostToolUse"
	// UserPromptSubmit fires when a user message opens a Run — a hook may
	// inject context or block the prompt.
	UserPromptSubmit Event = "UserPromptSubmit"
	// SessionStart fires on the first Run of a Session — a hook may inject
	// session-scoped context.
	SessionStart Event = "SessionStart"
	// SubagentStart fires after a delegated sub-agent Run is durably admitted.
	SubagentStart Event = "SubagentStart"
	// SubagentStop fires when a delegated sub-agent Run reaches a terminal
	// state.
	SubagentStop Event = "SubagentStop"
	// PreCompact fires before Run-boundary compaction — a hook may inject
	// guidance or veto the compaction.
	PreCompact Event = "PreCompact"
	// Stop fires at Run end (any terminal) — observe-only (notify / chain).
	Stop Event = "Stop"
	// Notification fires when a run parks waiting on the user (HITL interrupt)
	// — observe-only (route to an external sink).
	Notification Event = "Notification"
)

// Scope is where a hook came from — set by the loader, not the JSON. It gates
// trust: global hooks (the user's own ~/.lyra) always run; project hooks (a
// repo's .lyra, which a `git clone` could carry) run only for a trusted project.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
)

// Hook is one configured hook entry. Command and Inject are alternatives: a
// Command is exec'd (real logic); an Inject is a literal context string added
// with no process spawn (the declarative fast path). A Matcher (tool-name glob)
// applies only to tool events; configuration rejects it for other events.
type Hook struct {
	Event         Event
	Matcher       string
	Command       string
	Inject        string
	TimeoutMillis int

	// Scope + Source are stamped by the loader (provenance + trust gating), not
	// parsed from the file.
	Scope  Scope
	Source string
}

// ErrInvalidHook reports a malformed hook configuration. Invalid policy hooks
// must fail discovery instead of silently becoming no-ops.
var ErrInvalidHook = errors.New("hooks: invalid hook")

// ErrConfigurationTooLarge reports a hooks.json document or resolved cascade
// that cannot enter the complete management and execution projections.
var ErrConfigurationTooLarge = errors.New("hooks: configuration too large")

// ErrCommandInputTooLarge reports semantic invocation material that cannot be
// projected into the private command contract without changing policy input.
var ErrCommandInputTooLarge = errors.New("hooks: command input too large")

// ErrInvalidCommandInput reports command material that cannot enter the
// private JSON contract without silent text replacement.
var ErrInvalidCommandInput = errors.New("hooks: invalid command input")

// ValidateConfigurationFileSize checks the encoded envelope before a loader
// allocates or decodes one authored policy document.
func ValidateConfigurationFileSize(size int64) error {
	if size < 0 || size > MaxConfigurationFileBytes {
		return fmt.Errorf(
			"%w: file uses %d bytes, maximum %d",
			ErrConfigurationTooLarge,
			size,
			MaxConfigurationFileBytes,
		)
	}
	return nil
}

// ValidateHooksPerFile checks the complete entry count of one hooks.json.
func ValidateHooksPerFile(count int) error {
	if count < 0 || count > MaxHooksPerFile {
		return fmt.Errorf(
			"%w: file contains %d hooks, maximum %d",
			ErrConfigurationTooLarge,
			count,
			MaxHooksPerFile,
		)
	}
	return nil
}

// ValidateHookCascade checks the complete global + project policy set.
func ValidateHookCascade(count int) error {
	if count < 0 || count > MaxHooksPerCascade {
		return fmt.Errorf(
			"%w: cascade contains %d hooks, maximum %d",
			ErrConfigurationTooLarge,
			count,
			MaxHooksPerCascade,
		)
	}
	return nil
}

// Validate checks the declarative hook contract before a resolved set can be
// installed. A hook has one known lifecycle event and exactly one action;
// malformed matchers and negative timeouts are configuration errors rather
// than policies that quietly never run.
func (h Hook) Validate() error {
	switch h.Event {
	case PreToolUse, PostToolUse, UserPromptSubmit, SessionStart,
		SubagentStart, SubagentStop, PreCompact, Stop, Notification:
	default:
		return fmt.Errorf("%w: unsupported event %q", ErrInvalidHook, h.Event)
	}
	hasCommand := strings.TrimSpace(h.Command) != ""
	hasInject := strings.TrimSpace(h.Inject) != ""
	if hasCommand == hasInject {
		return fmt.Errorf("%w: exactly one of command or inject is required", ErrInvalidHook)
	}
	if !utf8.ValidString(h.Matcher) || !utf8.ValidString(h.Command) || !utf8.ValidString(h.Inject) {
		return fmt.Errorf("%w: text must be valid UTF-8", ErrInvalidHook)
	}
	if len(h.Matcher) > MaxMatcherBytes {
		return fmt.Errorf("%w: matcher exceeds %d bytes", ErrInvalidHook, MaxMatcherBytes)
	}
	if len(h.Command) > MaxActionBytes || len(h.Inject) > MaxActionBytes {
		return fmt.Errorf("%w: action exceeds %d bytes", ErrInvalidHook, MaxActionBytes)
	}
	if h.TimeoutMillis < 0 || h.TimeoutMillis > MaxTimeoutMillis {
		return fmt.Errorf(
			"%w: timeoutMillis must be between 0 and %d",
			ErrInvalidHook,
			MaxTimeoutMillis,
		)
	}
	if hasInject && h.TimeoutMillis != 0 {
		return fmt.Errorf("%w: timeoutMillis is only valid for command hooks", ErrInvalidHook)
	}
	if h.Matcher != "" {
		if h.Event != PreToolUse && h.Event != PostToolUse {
			return fmt.Errorf("%w: matcher is only valid for tool events", ErrInvalidHook)
		}
		if _, err := path.Match(h.Matcher, ""); err != nil {
			return fmt.Errorf("%w: invalid matcher %q: %w", ErrInvalidHook, h.Matcher, err)
		}
	}
	return nil
}

// Input is the semantic lifecycle payload passed to a hook runner.
type Input struct {
	Event           Event
	SessionID       string
	CWD             string
	Tool            *ToolInput
	Subagent        *SubagentInput
	Prompt          string
	PromptTruncated bool
	// Reason carries a human-readable note for the observe-only events (the Stop
	// terminal detail, the Notification reason).
	Reason string
}

// ToolInput is the tool slice of an Input for the tool events.
type ToolInput struct {
	Name            string
	Arguments       string // canonical JSON args (Pre/PostToolUse)
	Result          string // tool output (PostToolUse)
	ResultTruncated bool
}

// SubagentStatus is the stable terminal vocabulary exposed to lifecycle hooks.
type SubagentStatus string

const (
	SubagentCompleted  SubagentStatus = "completed"
	SubagentFailed     SubagentStatus = "failed"
	SubagentKilled     SubagentStatus = "killed"
	SubagentTerminated SubagentStatus = "terminated"
	SubagentStuck      SubagentStatus = "stuck"
)

// Valid reports whether s is a known sub-agent terminal status.
func (s SubagentStatus) Valid() bool {
	switch s {
	case SubagentCompleted, SubagentFailed, SubagentKilled, SubagentTerminated, SubagentStuck:
		return true
	default:
		return false
	}
}

// SubagentInput is the sub-agent slice of an Input for SubagentStart/Stop.
type SubagentInput struct {
	RunID           string
	ParentRunID     string
	Description     string
	Prompt          string
	PromptTruncated bool
	Status          SubagentStatus
	Result          string
	Error           string
	ResultTruncated bool
}

// CommandProjection returns an ownership-isolated, bounded view for external
// command hooks. Human-readable prompt/result material may use an explicit
// marked prefix; canonical Tool arguments never truncate because doing so would
// describe a different effect to policy code.
func (in Input) CommandProjection() (Input, error) {
	out := in
	out.Prompt, out.PromptTruncated = boundedCommandText(in.Prompt, MaxPromptBytes, in.PromptTruncated)
	out.Reason, _ = boundedCommandText(in.Reason, MaxReasonBytes, false)
	if in.Tool != nil {
		if len(in.Tool.Arguments) > MaxArgumentsBytes {
			return Input{}, fmt.Errorf(
				"%w: tool arguments use %d bytes, maximum %d",
				ErrCommandInputTooLarge,
				len(in.Tool.Arguments),
				MaxArgumentsBytes,
			)
		}
		tool := *in.Tool
		tool.Result, tool.ResultTruncated = boundedCommandText(
			in.Tool.Result,
			MaxResultBytes,
			in.Tool.ResultTruncated,
		)
		out.Tool = &tool
	}
	if in.Subagent != nil {
		subagent := *in.Subagent
		subagent.Description, _ = boundedCommandText(in.Subagent.Description, MaxReasonBytes, false)
		subagent.Prompt, subagent.PromptTruncated = boundedCommandText(
			in.Subagent.Prompt,
			MaxPromptBytes,
			in.Subagent.PromptTruncated,
		)
		subagent.Result, subagent.ResultTruncated = boundedCommandText(
			in.Subagent.Result,
			MaxResultBytes,
			in.Subagent.ResultTruncated,
		)
		subagent.Error, _ = boundedCommandText(in.Subagent.Error, MaxReasonBytes, false)
		out.Subagent = &subagent
	}
	if err := out.ValidateCommandMaterial(); err != nil {
		return Input{}, err
	}
	return out, nil
}

// ValidateCommandMaterial is the process-boundary backstop for callers that
// bypass CommandProjection. It rejects lossy or oversized semantic input before
// JSON encoding and process creation.
func (in Input) ValidateCommandMaterial() error {
	if len(in.Prompt) > MaxPromptBytes || len(in.Reason) > MaxReasonBytes {
		return ErrCommandInputTooLarge
	}
	values := []string{string(in.Event), in.SessionID, in.CWD, in.Prompt, in.Reason}
	if in.Tool != nil {
		if len(in.Tool.Arguments) > MaxArgumentsBytes || len(in.Tool.Result) > MaxResultBytes {
			return ErrCommandInputTooLarge
		}
		values = append(values, in.Tool.Name, in.Tool.Arguments, in.Tool.Result)
	}
	if in.Subagent != nil {
		if len(in.Subagent.Description) > MaxReasonBytes ||
			len(in.Subagent.Prompt) > MaxPromptBytes ||
			len(in.Subagent.Result) > MaxResultBytes ||
			len(in.Subagent.Error) > MaxReasonBytes {
			return ErrCommandInputTooLarge
		}
		values = append(
			values,
			in.Subagent.RunID,
			in.Subagent.ParentRunID,
			in.Subagent.Description,
			in.Subagent.Prompt,
			string(in.Subagent.Status),
			in.Subagent.Result,
			in.Subagent.Error,
		)
	}
	for _, value := range values {
		if !utf8.ValidString(value) {
			return fmt.Errorf("%w: material must be valid UTF-8", ErrInvalidCommandInput)
		}
	}
	return nil
}

func boundedCommandText(value string, limit int, alreadyTruncated bool) (string, bool) {
	if limit <= 0 {
		return "", alreadyTruncated || value != ""
	}
	truncated := alreadyTruncated || len(value) > limit
	if len(value) > limit {
		value = value[:limit]
	}
	valid := strings.ToValidUTF8(value, "�")
	if valid != value {
		truncated = true
	}
	if len(valid) > limit {
		valid = valid[:limit]
		for len(valid) > 0 && !utf8.ValidString(valid) {
			valid = valid[:len(valid)-1]
		}
		truncated = true
	}
	return valid, truncated
}

// Decision is the combined verdict of every hook that fired for one event.
type Decision struct {
	// Block denies the action (the tool, or the prompt). Reason is fed to the
	// model so it knows why.
	Block  bool
	Reason string
	// Ask forces an approval prompt for a PreToolUse the gate would otherwise
	// pass (a hook escalating to human review). Ignored once Block is set.
	Ask bool
	// InjectContext is extra context to surface (concatenated across hooks).
	InjectContext string
	// RewriteArguments, when set (PreToolUse), replaces the tool's arguments
	// (raw JSON). First non-empty wins.
	RewriteArguments string
}

// Matches reports whether h applies to in.
func (h Hook) Matches(in Input) bool {
	if h.Event != in.Event {
		return false
	}
	if in.Event != PreToolUse && in.Event != PostToolUse {
		return true // non-tool events: matcher is irrelevant
	}
	if h.Matcher == "" {
		return true
	}
	name := ""
	if in.Tool != nil {
		name = in.Tool.Name
	}
	ok, err := path.Match(h.Matcher, name)
	return err == nil && ok
}

// Fold combines one matching hook outcome using first-deny and first-rewrite
// precedence while accumulating injected context.
func (d *Decision) Fold(block, ask bool, reason, inject, rewrite string) {
	if block && !d.Block {
		d.Block = true
		d.Reason = reason
	}
	if ask && !d.Block {
		d.Ask = true
		if d.Reason == "" {
			d.Reason = reason
		}
	}
	if inject != "" {
		if d.InjectContext != "" {
			d.InjectContext += "\n"
		}
		d.InjectContext += inject
	}
	if rewrite != "" && d.RewriteArguments == "" {
		d.RewriteArguments = rewrite
	}
}
