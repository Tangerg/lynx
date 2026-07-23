// Package approval defines the runtime tool-call approval policy. Two concerns
// live here:
//
//   - Mode: the runtime-wide stance (plan / safe / balanced / yolo) the chat
//     engine reads at each tool call to decide whether a call runs, is denied,
//     or must pause for approval. The HITL pause/resume is the R model (the
//     agent runtime parks a Suspension, the client answers via runs.resume) —
//     see internal/adapter/agentexec/turn + internal/domain/execution/interrupts.
//   - Rules: persistent, fine-grained "remember this decision" rules. A rule
//     gates a (tool, subject) pair under a scope (session / project / global),
//     so the user can approve once and not be re-asked for matching calls. The
//     subject is the per-tool part that actually matters — a shell command, an
//     edited file's path — so a rule is "allow `npm run *` in this project",
//     not the blunt "allow every shell call ever".
package approval

import (
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

var (
	ErrInvalidMode          = errors.New("approval: invalid mode")
	ErrInvalidQuery         = errors.New("approval: invalid query")
	ErrInvalidRule          = errors.New("approval: invalid rule")
	ErrRuleStoreUnavailable = errors.New("approval: rule store unavailable")
)

// Mode is the runtime-wide permission stance. Set via config or the
// approval.setMode method; read at each tool call by the chat
// engine's approval gate.
//
// Strictness gradient (strictest → loosest):
//
//	ModePlan      read-only: deny every write / exec / network tool (no prompt)
//	ModeSafe      prompt on every write / exec / network tool
//	ModeBalanced  auto-allow write/network; prompt only on exec
//	ModeYolo      auto-allow everything
//
// The const VALUES are not in strictness order — ModePlan is appended
// (value 3) so the existing zero value (ModeSafe) is unchanged. Order
// code against the named constants, never the ints.
type Mode int

const (
	// ModeSafe — every Exec/Write/Network tool prompts.
	ModeSafe Mode = iota
	// ModeBalanced — Write/Network auto-allow; Exec prompts.
	ModeBalanced
	// ModeYolo — auto-allow everything (use at your own risk).
	ModeYolo
	// ModePlan — the read-only planning stance: every write / exec /
	// network tool is denied outright (no prompt) so the agent can only
	// investigate and draft a plan; the model sees the refusal as a tool
	// error and adapts. The exit_plan_mode tool presents the plan for
	// approval and flips the stance back to execute (ModeBalanced).
	ModePlan
)

func (m Mode) Valid() bool {
	switch m {
	case ModeSafe, ModeBalanced, ModeYolo, ModePlan:
		return true
	default:
		return false
	}
}

// Scope is how far a remembered rule reaches.
type Scope string

const (
	// ScopeSession — the rule lives only for one session (keyed by session id).
	ScopeSession Scope = "session"
	// ScopeProject — the rule applies to every session opened in one project
	// directory (keyed by that cwd).
	ScopeProject Scope = "project"
	// ScopeGlobal — the rule applies everywhere (no key).
	ScopeGlobal Scope = "global"
)

func (s Scope) Valid() bool {
	switch s {
	case ScopeSession, ScopeProject, ScopeGlobal:
		return true
	default:
		return false
	}
}

// Decision is a rule's standing verdict.
type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
)

func (d Decision) Valid() bool { return d == Allow || d == Deny }

// Rule is one standing approval decision. A rule matches a tool call when the
// call's scope key matches (same session / same project dir / always for
// global), the tool name matches, and the call's per-tool subject matches the
// Subject glob (empty Subject = any arguments for that tool).
type Rule struct {
	ID       string   // deterministic over (Scope, ScopeKey, Tool, Subject)
	Scope    Scope    // session | project | global
	ScopeKey string   // session id | project dir | "" for global
	Tool     string   // tool name, e.g. "shell"
	Subject  string   // glob over the call's subject (command / path); "" = any
	Decision Decision // allow | deny
}

// Query identifies one gated tool call for [Policy.Decide]. ProjectDir is the
// call's working directory (the project scope key); empty for sessions without
// a cwd. Arguments owns the validated call object used to derive its subject.
type Query struct {
	SessionID  string
	ProjectDir string
	Tool       string
	Arguments  tool.Arguments
}

// RememberRequest persists a rule from a user's "approve/deny + remember{scope}"
// choice. The subject is extracted from Arguments per the tool, so the rule
// matches future calls like this one — not the blunt whole-tool grant.
type RememberRequest struct {
	Scope      Scope
	SessionID  string
	ProjectDir string
	Tool       string
	Arguments  tool.Arguments
	Decision   Decision
}
