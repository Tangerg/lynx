// Package approval defines the ApprovalService — Lyra's tool-call
// permission decision surface. Approval requests fire as part of a
// chat turn (via the ToolCallApproval event, added M4); clients reply
// asynchronously via Decide.
package approval

import (
	"context"
	"time"
)

// Mode is the runtime-wide permission stance. Set via config; flows
// into the per-tool gate decision at request time.
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
	// ModeReadOnly — the strictest stance: only read-only tools
	// (read / grep / glob) run; every write / exec / network tool is
	// denied immediately WITHOUT prompting, and the model sees the
	// refusal as a tool error so it can adapt. Use for "explore /
	// explain my code but touch nothing" sessions and untrusted
	// contexts. Distinct from plan-mode (which previews then
	// executes) and ModeSafe (which prompts rather than denies).
	ModeReadOnly
)

// Request is one outstanding approval ask. Created by the runtime
// when a tool call needs gating; resolved by [Service.Decide].
type Request struct {
	ID          string
	SessionID   string
	TurnID      string
	ToolName    string
	Arguments   string // raw JSON the model emitted
	RequestedAt time.Time
}

// Decision is the outcome the client returns for a pending request.
// Two values only, mirroring the wire contract (runs.approval.submit
// carries "approve" | "deny"). "Remember this choice" / "always
// allow" is deliberately NOT modeled here — per the frontend
// protocol alignment it is a client-side UI affordance, not a wire
// or backend concept; the backend only ever sees approve or deny.
type Decision int

const (
	// DecisionApprove — let this call through.
	DecisionApprove Decision = iota
	// DecisionDeny — abort this call. The tool returns an error to
	// the model so the model can recover.
	DecisionDeny
)

// Console is the client-side surface of approvals: list what's
// pending, push a verdict, read or change the runtime stance.
// HTTP handlers + the CLI depend on this, NOT on Gate.
type Console interface {
	// ListPending returns every unresolved approval request — useful
	// for client startup ("any approvals waiting for me?").
	ListPending(ctx context.Context) ([]Request, error)

	// Decide resolves a pending request. Returns [ErrRequestNotFound]
	// when no pending request matches requestID — either the id is
	// bogus or the request has already been decided.
	Decide(ctx context.Context, requestID string, decision Decision) error

	// SetMode changes the runtime-wide stance. Future tool calls
	// honor the new mode; in-flight calls keep their original mode.
	SetMode(ctx context.Context, mode Mode) error

	// GetMode returns the current runtime stance. Producers
	// (chat / engine) also call this to decide whether a tool needs
	// gating — hence its presence on both Console and Gate via
	// the Service union.
	GetMode(ctx context.Context) (Mode, error)
}

// Gate is the producer-side surface: declare an ask, get the
// decision channel, clean up. The chat service depends on this,
// NOT on Console.
//
// The split — register / emit / wait — exists so producers can
// emit the user-facing event AFTER registration completes, with
// no race where [Console.Decide] could be called before the
// pending entry is observable to [Console.ListPending].
type Gate interface {
	// GetMode lets producers short-circuit gating (e.g. ModeYolo
	// auto-passes everything). Mirrored from Console — same signal,
	// both sides need it.
	GetMode(ctx context.Context) (Mode, error)

	// Register declares a pending approval ask and returns the
	// channel its decision lands on, plus a cleanup func the
	// caller MUST call (typically via defer) to remove the entry
	// if the caller gives up before [Console.Decide] arrives.
	//
	// req.ID must be non-empty and globally unique within the
	// process; producers typically reuse the tool call's UUID so
	// the request id matches the lifecycle event ids on the same
	// turn channel.
	Register(req Request) (<-chan Decision, func())
}

// Service is the union of [Console] + [Gate] — kept so a single
// concrete implementation can satisfy both roles and runtime
// wiring stays a single object. Per ISP, callers should depend on
// the narrowest interface they actually use.
type Service interface {
	Console
	Gate
}
