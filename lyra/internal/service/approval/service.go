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
// into the per-tool decision at request time.
type Mode int

const (
	// ModeSafe — every Exec/Write/Network tool prompts.
	ModeSafe Mode = iota
	// ModeBalanced — Write/Network auto-allow; Exec prompts (with
	// optional LLM classifier in M-future).
	ModeBalanced
	// ModeYolo — auto-allow everything (use at your own risk).
	ModeYolo
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
type Decision int

const (
	// DecisionAllowOnce — let this one call through.
	DecisionAllowOnce Decision = iota
	// DecisionAllowAlways — let this one through and cache the
	// (tool, normalized-args) pair so future identical calls auto-pass.
	DecisionAllowAlways
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
	// honour the new mode; in-flight calls keep their original mode.
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
