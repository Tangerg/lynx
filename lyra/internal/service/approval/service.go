// Package approval defines the ApprovalService — Lyra's runtime
// tool-call permission stance. It is now a small mode holder: the chat
// engine reads the mode to decide whether a tool call runs, is denied,
// or must pause for user approval. The actual HITL pause/resume is the
// R model (the agent runtime parks the process on AwaitInput and the
// client answers via runs.resume) — see internal/service/chat +
// internal/service/interrupts. There is no blocking
// register/decide/decision-channel machinery here anymore.
package approval

import "context"

// Mode is the runtime-wide permission stance. Set via config; read at
// each tool call by the chat engine's approval gate.
//
// Strictness gradient (strictest → loosest):
//
//	ModeReadOnly  deny every non-read tool outright (no prompt)
//	ModeSafe      prompt on every write / exec / network tool
//	ModeBalanced  auto-allow write/network; prompt only on exec
//	ModeYolo      auto-allow everything
//
// The const VALUES are not in strictness order — ModeReadOnly is
// appended (value 3) so the existing zero value (ModeSafe) is
// unchanged. Order code against the named constants, never the ints.
type Mode int

const (
	// ModeSafe — every Exec/Write/Network tool prompts.
	ModeSafe Mode = iota
	// ModeBalanced — Write/Network auto-allow; Exec prompts.
	ModeBalanced
	// ModeYolo — auto-allow everything (use at your own risk).
	ModeYolo
	// ModeReadOnly — strictest: only read-only tools run; every
	// write / exec / network tool is denied immediately without
	// prompting, and the model sees the refusal as a tool error so it
	// can adapt.
	ModeReadOnly
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
}
