package client

import (
	"context"
	"iter"
)

// Stream is a replayable run subscription. Every envelope after the requested
// cursor is delivered in cursor order. A subscription ends when the run
// finishes or interrupts, or yields one non-nil error and stops.
type Stream = iter.Seq2[Envelope, error]

// Runtime is the complete capability assembled at the application boundary.
// Consumers depend on the narrower interfaces below.
type Runtime interface {
	SessionCatalog
	SessionReader
	SessionWriter
	Runs
	Models
}

// SessionCatalog discovers sessions without loading their transcripts.
type SessionCatalog interface {
	ListSessions(context.Context, SessionQuery) (SessionPage, error)
}

// SessionReader restores one session and its authoritative event history.
type SessionReader interface {
	GetSession(context.Context, string) (SessionSnapshot, error)
}

// SessionWriter owns session lifecycle mutations.
type SessionWriter interface {
	CreateSession(context.Context, NewSession) (Session, error)
	UpdateSession(context.Context, UpdateSession) (Session, error)
	ForkSession(context.Context, ForkSession) (Session, error)
	DeleteSession(context.Context, DeleteSession) error
}

// Runs controls logical runs independently from their transport subscriptions.
// A disconnected subscriber can call FollowRun again with the last accepted
// cursor without restarting or duplicating the run.
type Runs interface {
	StartRun(context.Context, StartRun) (Run, error)
	FollowRun(context.Context, FollowRun) (Stream, error)
	ResumeRun(context.Context, ResumeRun) error
	CancelRun(context.Context, string) error
}

// Models exposes the runtime-provided model catalog. Run modes and permission
// modes remain CLI product concepts and are sent back in RunOptions.
type Models interface {
	ListModels(context.Context) ([]Model, error)
}

// StartRun describes a new user turn.
type StartRun struct {
	SessionID string
	Message   Message
	Options   RunOptions
}

// FollowRun subscribes after a cursor previously accepted by the client.
type FollowRun struct {
	RunID string
	After Cursor
}

// ResumeRun answers the interrupt currently holding a run.
type ResumeRun struct {
	RunID       string
	InterruptID string
	Answer      Answer
}
