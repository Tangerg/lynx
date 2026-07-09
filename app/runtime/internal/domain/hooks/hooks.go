// Package hooks is Lyra's user-configurable lifecycle hooks: at fixed points in
// a turn (before/after a tool, at prompt submit, session start, compaction,
// turn end, and when waiting on the user) the runtime runs user-authored hooks
// and lets them observe, block, or rewrite what happens next.
//
// Design — why subprocess, not an embedded script VM (see doc): a hook is an
// external COMMAND (any language) invoked with the event as JSON on stdin; it
// answers with an exit code (+ optional JSON on stdout). This is a host-language-
// agnostic contract — the same model Claude Code uses — so being a Go runtime is
// no disadvantage (Go orchestrates processes well), it composes with a future OS
// sandbox, and users write hooks in whatever they like. A declarative `inject`
// covers the common "add this context" case with zero process spawn; the matcher
// gates exec so an unrelated tool never forks a hook. There is deliberately NO
// embedded interpreter (Goja/Starlark/…): that's weight + a security surface the
// subprocess contract doesn't need.
//
// This package is the pure hook core: lifecycle types, matching, and decision
// combination. Filesystem discovery and shell execution live in adapter/hooks.
package hooks

import (
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
// applies to the tool events; it's ignored for the others.
type Hook struct {
	Event     Event  `json:"event"`
	Matcher   string `json:"matcher,omitempty"`
	Command   string `json:"command,omitempty"`
	Inject    string `json:"inject,omitempty"`
	TimeoutMs int    `json:"timeoutMs,omitempty"`

	// Scope + Source are stamped by the loader (provenance + trust gating), not
	// parsed from the file.
	Scope  Scope  `json:"-"`
	Source string `json:"-"`
}

// Input is the event payload handed to a hook on stdin (JSON).
type Input struct {
	Event     Event          `json:"event"`
	SessionID string         `json:"sessionId,omitempty"`
	Cwd       string         `json:"cwd,omitempty"`
	Tool      *ToolInput     `json:"tool,omitempty"`
	Subagent  *SubagentInput `json:"subagent,omitempty"`
	Prompt    string         `json:"prompt,omitempty"`
	// Reason carries a human-readable note for the observe-only events (the Stop
	// terminal detail, the Notification reason).
	Reason string `json:"reason,omitempty"`
}

// ToolInput is the tool slice of an Input for the tool events.
type ToolInput struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"` // raw JSON args (PreToolUse)
	Result    string `json:"result,omitempty"`    // tool output (PostToolUse)
}

// SubagentInput is the sub-agent slice of an Input for SubagentStart/Stop.
type SubagentInput struct {
	ProcessID       string `json:"processId"`
	ParentProcessID string `json:"parentProcessId,omitempty"`
	Description     string `json:"description,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
	Status          string `json:"status,omitempty"`
	Result          string `json:"result,omitempty"`
	Error           string `json:"error,omitempty"`
}

// SubagentTask is the optional shape a delegated task input can implement so
// lifecycle hooks receive a stable display summary without reflection.
type SubagentTask interface {
	SubagentDescription() string
	SubagentPrompt() string
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
// tool name. An empty matcher matches every tool. A malformed glob never
// matches (it's the hook author's bug, not a reason to fire on everything).
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
type hookOutput struct {
	Decision         string `json:"decision,omitempty"` // "allow" | "deny" | "ask"
	Reason           string `json:"reason,omitempty"`
	InjectContext    string `json:"injectContext,omitempty"`
	RewriteArguments string `json:"rewriteArguments,omitempty"`
}

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

// trimZero collapses an all-whitespace string to "".
func trimZero(s string) string { return strings.TrimSpace(s) }
