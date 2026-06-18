// Package approval defines the ApprovalService — Lyra's runtime
// tool-call permission stance. It is now a small mode holder: the chat
// engine reads the mode to decide whether a tool call runs, is denied,
// or must pause for user approval. The actual HITL pause/resume is the
// R model (the agent runtime parks the process on AwaitInput and the
// client answers via runs.resume) — see internal/kernel/turn +
// internal/domain/interrupts. There is no blocking
// register/decide/decision-channel machinery here anymore.
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

// Service is the runtime approval stance. Read at each tool call by the
// chat engine; mutable at runtime so a future control surface can flip
// the stance mid-session. All methods are safe for concurrent use.
type Service interface {
	// GetMode returns the current runtime stance.
	GetMode(ctx context.Context) (Mode, error)

	// SetMode changes the runtime-wide stance. Future tool calls honor
	// the new mode; in-flight calls keep their original mode.
	SetMode(ctx context.Context, mode Mode) error

	// Remember records a standing per-session decision for a tool, so future
	// gated calls to it in that session skip the prompt (AUX_API §6,
	// "approve/deny + remember"). approved=true auto-passes; approved=false
	// auto-denies — recording a denial is a valid choice. The key is the tool
	// NAME (not its arguments); scope is the session. v1 is in-memory and
	// resets on restart — project / global scopes aren't persisted yet.
	Remember(ctx context.Context, sessionID, toolName string, approved bool) error

	// Remembered reports a standing session decision for a tool: ok=false when
	// none was recorded, otherwise approved is the recorded verdict. The chat
	// gate consults it before prompting so a remembered tool never re-asks.
	Remembered(ctx context.Context, sessionID, toolName string) (approved bool, ok bool, err error)
}
