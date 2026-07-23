// Package hooks is Lyra's user-configurable lifecycle hooks: at fixed points in
// a turn (before/after a tool, at prompt submit, session start, compaction,
// turn end, and when waiting on the user) the runtime runs user-authored hooks
// and lets them observe, block, or rewrite what happens next.
//
// Design — why subprocess, not an embedded script VM (see doc): a hook is an
// external COMMAND (any language) invoked with the event as JSON on stdin; it
// answers with an exit code (+ optional JSON on stdout). This is a host-language-
// agnostic contract — the same model Claude Code uses — so being a Go runtime is
// no disadvantage (Go orchestrates processes well), and users write hooks in
// whatever they like. A declarative `inject`
// covers the common "add this context" case with zero process spawn; the matcher
// gates an action so an unrelated tool never runs it. There is deliberately NO
// embedded interpreter (Goja/Starlark/…): that's weight + a security surface the
// subprocess contract doesn't need.
//
// This package is the pure hook core: lifecycle types, matching, and decision
// combination. Filesystem discovery and shell execution live in adapter/hooks.
package hooks

import (
	"errors"
	"fmt"
	"path"
	"strings"
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
	// UserPromptSubmit fires when a user message opens a turn — a hook may
	// inject context or block the prompt.
	UserPromptSubmit Event = "UserPromptSubmit"
	// SessionStart fires on the first turn of a session — a hook may inject
	// session-scoped context.
	SessionStart Event = "SessionStart"
	// SubagentStart fires when a delegated sub-agent process starts.
	SubagentStart Event = "SubagentStart"
	// SubagentStop fires when a delegated sub-agent process reaches a terminal
	// state. Reason carries the agent runtime terminal event name.
	SubagentStop Event = "SubagentStop"
	// PreCompact fires before turn-boundary compaction — a hook may inject
	// guidance or veto the compaction.
	PreCompact Event = "PreCompact"
	// Stop fires at turn end (any terminal) — observe-only (notify / chain).
	Stop Event = "Stop"
	// Notification fires when a run parks waiting on the user (HITL interrupt)
	// — observe-only (route to Slack / desktop / etc.).
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
	Event     Event
	Matcher   string
	Command   string
	Inject    string
	TimeoutMs int

	// Scope + Source are stamped by the loader (provenance + trust gating), not
	// parsed from the file.
	Scope  Scope
	Source string
}

// ErrInvalidHook reports a malformed hook configuration. Invalid policy hooks
// must fail discovery instead of silently becoming no-ops.
var ErrInvalidHook = errors.New("hooks: invalid hook")

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
	if h.TimeoutMs < 0 {
		return fmt.Errorf("%w: timeoutMs must be non-negative", ErrInvalidHook)
	}
	if hasInject && h.TimeoutMs != 0 {
		return fmt.Errorf("%w: timeoutMs is only valid for command hooks", ErrInvalidHook)
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

// Input is the event payload handed to a hook on stdin (JSON).
type Input struct {
	Event     Event
	SessionID string
	Cwd       string
	Tool      *ToolInput
	Subagent  *SubagentInput
	Prompt    string
	// Reason carries a human-readable note for the observe-only events (the Stop
	// terminal detail, the Notification reason).
	Reason string
}

// ToolInput is the tool slice of an Input for the tool events.
type ToolInput struct {
	Name      string
	Arguments string // raw JSON args (PreToolUse)
	Result    string // tool output (PostToolUse)
}

// SubagentInput is the sub-agent slice of an Input for SubagentStart/Stop.
type SubagentInput struct {
	ProcessID       string
	ParentProcessID string
	Description     string
	Prompt          string
	Status          string
	Result          string
	Error           string
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

// matches reports whether hook h should fire for input in: the events must
// match, and for the tool events the hook's matcher (a glob) must match the
// tool name. An empty matcher matches every tool. Discovery rejects malformed
// globs; matches still treats one defensively as non-matching.
func (h Hook) matches(in Input) bool {
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

// hookOutput is the optional JSON a hook prints to stdout to control the
// outcome beyond the exit code.

// fold merges one hook's outcome into the running decision. block latches (the
// first denying hook owns the reason); ask is a softer escalation; injected
// context concatenates; the first rewrite wins (deterministic).
func (d *Decision) fold(block, ask bool, reason, inject, rewrite string) {
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
