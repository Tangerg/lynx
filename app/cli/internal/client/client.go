package client

import (
	"context"
	"iter"
)

// Stream is a run's events in order, ending either at a [RunFinished], at a
// [RunParked], or at the first error. A stream yields a nil Event with a
// non-nil error and stops; it never yields both.
type Stream = iter.Seq2[Event, error]

// Runtime is everything the CLI needs from a lyra runtime. It exists to be
// assembled from the narrower groups below, not to be depended on: a command
// takes the group it uses.
type Runtime interface {
	Sessions
	Runs
}

// Sessions is the session catalogue.
type Sessions interface {
	// ListSessions returns sessions most recently touched first.
	//
	// Backed by sessions.list. Paging is the implementation's business: a
	// terminal asks for "the sessions", not for a page of them.
	ListSessions(ctx context.Context) ([]Session, error)

	// CreateSession opens a new session in a workspace.
	//
	// Backed by sessions.create.
	CreateSession(ctx context.Context, in NewSession) (Session, error)
}

// Runs drives a session's work.
type Runs interface {
	// StartRun begins a run and returns its event stream. The stream ends at a
	// [RunFinished] or at a [RunParked]; a parked run is still alive and is
	// continued with ResumeRun.
	//
	// Backed by runs.start, whose response is itself a stream. Reports
	// [ErrSessionNotFound] for a session that does not exist.
	StartRun(ctx context.Context, in StartRun) (Stream, error)

	// ResumeRun answers what a run parked on and returns the continuation
	// stream. The run id stays the same across the park; only the stream is new.
	//
	// Backed by runs.resume. Reports [ErrRunNotFound] for an unknown run and
	// [ErrInterruptNotOpen] when the park has already been answered or the
	// interrupt id belongs to a different one.
	ResumeRun(ctx context.Context, in ResumeRun) (Stream, error)

	// CancelRun asks a run to stop. It returns once the request is lodged, not
	// once the run has stopped: the stream reports the stop, as a
	// [RunFinished] with [OutcomeCanceled].
	//
	// Backed by runs.cancel. Reports [ErrRunNotFound] for an unknown run.
	CancelRun(ctx context.Context, runID string) error
}

// NewSession describes a session to open.
type NewSession struct {
	// Title may be empty, in which case the runtime names the session.
	Title string
	// Workspace is the absolute path the session works in.
	Workspace string
}

// StartRun describes a run to begin.
type StartRun struct {
	SessionID string
	Prompt    string
}

// ResumeRun describes how a parked run continues.
type ResumeRun struct {
	RunID       string
	InterruptID string
	Decision    Decision
}
