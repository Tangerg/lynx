// Package approval defines the ApprovalService — Lyra's runtime tool-call
// permission stance. Two concerns live here:
//
//   - Mode: the runtime-wide stance (plan / safe / balanced / yolo) the chat
//     engine reads at each tool call to decide whether a call runs, is denied,
//     or must pause for approval. The HITL pause/resume is the R model (the
//     agent runtime parks on AwaitInput, the client answers via runs.resume) —
//     see internal/kernel/turn + internal/domain/interrupts.
//   - Rules: persistent, fine-grained "remember this decision" rules. A rule
//     gates a (tool, subject) pair under a scope (session / project / global),
//     so the user can approve once and not be re-asked for matching calls. The
//     subject is the per-tool part that actually matters — a bash command, an
//     edited file's path — so a rule is "allow `npm run *` in this project",
//     not the blunt "allow every bash call ever".
package approval

import "context"

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

// Decision is a rule's standing verdict.
type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
)

// Rule is one standing approval decision. A rule matches a tool call when the
// call's scope key matches (same session / same project dir / always for
// global), the tool name matches, and the call's per-tool subject matches the
// Subject glob (empty Subject = any arguments for that tool).
type Rule struct {
	ID       string   // deterministic over (Scope, ScopeKey, Tool, Subject)
	Scope    Scope    // session | project | global
	ScopeKey string   // session id | project dir | "" for global
	Tool     string   // tool name, e.g. "bash"
	Subject  string   // glob over the call's subject (command / path); "" = any
	Decision Decision // allow | deny
}

// Query identifies one gated tool call for [Service.Decide]. ProjectDir is the
// call's working directory (the project scope key); empty for sessions without
// a cwd. Arguments is the raw tool-call JSON — the subject is extracted from it.
type Query struct {
	SessionID  string
	ProjectDir string
	Tool       string
	Arguments  string
}

// RememberRequest persists a rule from a user's "approve/deny + remember{scope}"
// choice. The subject is extracted from Arguments per the tool, so the rule
// matches future calls like this one — not the blunt whole-tool grant.
type RememberRequest struct {
	Scope      Scope
	SessionID  string
	ProjectDir string
	Tool       string
	Arguments  string
	Decision   Decision
}

// Service is the runtime approval stance + the persistent rule store. Read at
// each tool call by the chat engine; mutable at runtime. All methods are safe
// for concurrent use.
type Service interface {
	// GetMode returns the current runtime stance.
	GetMode(ctx context.Context) (Mode, error)

	// SetMode changes the runtime-wide stance. Future tool calls honor the new
	// mode; in-flight calls keep their original mode.
	SetMode(ctx context.Context, mode Mode) error

	// Decide resolves a gated tool call against the stored rules. ok=false when
	// no rule matches (the gate then prompts the user). When several rules
	// match, the most specific wins (session > project > global, then exact
	// subject > glob > any); a tie between conflicting decisions resolves to
	// Deny (the conservative choice).
	Decide(ctx context.Context, q Query) (decision Decision, ok bool, err error)

	// Remember persists a rule from an approve/deny + remember choice. A project
	// rule with no ProjectDir, or a session rule with no SessionID, is dropped
	// rather than stored under an empty key.
	Remember(ctx context.Context, req RememberRequest) error

	// Rules lists every rule visible from a session — its session rules, its
	// project's rules, and all global rules — for the management UI.
	Rules(ctx context.Context, sessionID, projectDir string) ([]Rule, error)

	// Forget removes one rule by id.
	Forget(ctx context.Context, id string) error
}
