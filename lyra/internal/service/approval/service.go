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

// Service is the ApprovalService contract.
//
// The interface unifies two roles. Client-facing methods
// (ListPending, Decide, SetMode, GetMode) drive the UX layer —
// list outstanding asks, push a verdict, swap the runtime stance.
// Producer-side (Register) is used by chat / engine to declare a
// pending ask and obtain the channel the decision arrives on.
type Service interface {
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

	// GetMode returns the current runtime stance.
	GetMode(ctx context.Context) (Mode, error)

	// Register declares a pending approval ask and returns the
	// channel its decision lands on, plus a cleanup func the caller
	// MUST call (typically via defer) to remove the entry if the
	// caller gives up before [Decide] arrives.
	//
	// The split — register / emit / wait — exists so producers can
	// emit the user-facing event after registration completes, with
	// no race where [Decide] could be called before the pending
	// entry exists.
	//
	// req.ID must be non-empty and globally unique within the
	// process; producers typically reuse the tool call's UUID so
	// the request id matches the lifecycle event ids on the same
	// turn channel.
	Register(req Request) (<-chan Decision, func())
}
