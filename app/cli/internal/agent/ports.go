package agent

import (
	"context"
	"iter"
	"slices"
)

// EventStream is the ordered event stream of one run segment. A stream yields
// at most one non-nil error and then stops.
type EventStream = iter.Seq2[RunEvent, error]

// SegmentStream is an opened segment together with its event stream. Event IDs
// are opaque tokens scoped to SegmentID; callers may retain and return them but
// must never parse or order them.
type SegmentStream struct {
	RunID       string
	SegmentID   string
	UserItemID  string
	HeadEventID string
	Events      EventStream
}

// Runtime is the complete capability assembled at the application boundary.
// Use cases depend on the narrower consumer-owned interfaces below.
type Runtime interface {
	SessionCatalog
	SessionReader
	SessionWriter
	RunCatalog
	RunLifecycle
	ModelCatalog
	ApprovalCatalog
}

type SessionCatalog interface {
	ListSessions(context.Context, SessionQuery) (SessionPage, error)
}

// SessionReader returns a cold, authoritative projection assembled from the
// runtime's session, item, run, plan, and interrupt reads.
type SessionReader interface {
	GetSession(context.Context, string) (SessionSnapshot, error)
}

type SessionWriter interface {
	CreateSession(context.Context, CreateSession) (Session, error)
	UpdateSession(context.Context, UpdateSession) (Session, error)
	ForkSession(context.Context, ForkSession) (Session, error)
	RollbackSession(context.Context, RollbackSession) (RollbackResult, error)
	DeleteSession(context.Context, DeleteSession) error
}

// RunCatalog exposes durable run projections independently from a session
// transcript. It is the read side used by operational commands and recovery
// diagnostics that already hold a run identity.
type RunCatalog interface {
	GetRun(context.Context, string) (Run, error)
	ListRuns(context.Context, RunQuery) (RunPage, error)
}

// RunLifecycle opens and rebinds segment streams. StartRun and ResumeRun return
// the stream created by the same atomic runtime operation; consumers never have
// to race a second subscription against the first event.
type RunLifecycle interface {
	StartRun(context.Context, StartRun) (SegmentStream, error)
	ResumeRun(context.Context, ResumeRun) (SegmentStream, error)
	SubscribeRun(context.Context, SubscribeRun) (SegmentStream, error)
	SteerRun(context.Context, SteerRun) error
	CancelRun(context.Context, CancelRun) (RunCancellation, error)
}

// ModelCatalog exposes provider-qualified models. Model IDs are not assumed to
// be globally unique.
type ModelCatalog interface {
	ListModels(context.Context) ([]Model, error)
}

// ApprovalCatalog manages the runtime-wide approval stance and the remembered
// rules visible from a particular session.
type ApprovalCatalog interface {
	GetApprovalMode(context.Context) (ApprovalMode, error)
	SetApprovalMode(context.Context, ApprovalMode) (ApprovalMode, error)
	ListApprovalRules(context.Context, string) ([]ApprovalRule, error)
	DeleteApprovalRule(context.Context, string) error
}

type StartRun struct {
	CommandID CommandID
	SessionID string
	Message   Message
	Options   RunOptions
}

func (s StartRun) Clone() StartRun {
	s.Message = s.Message.Clone()
	s.Options = s.Options.Clone()
	return s
}

func (s StartRun) Equal(other StartRun) bool {
	return s.CommandID == other.CommandID && s.SessionID == other.SessionID &&
		s.Message.Equal(other.Message) && s.Options.Equal(other.Options)
}

// SubscribeRun rebinds one exact segment. AfterEventID is an opaque checkpoint
// previously accepted from that segment. Empty means attach at its current head.
type SubscribeRun struct {
	RunID        string
	SegmentID    string
	AfterEventID string
}

// InterruptAnswer pairs a response with the pending item it answers. A resume
// consumes the complete waiting set atomically.
type InterruptAnswer struct {
	ItemID string
	Answer Answer
}

type ResumeRun struct {
	CommandID CommandID
	RunID     string
	Answers   []InterruptAnswer
	Message   *Message
}

// Equal reports whether two resume commands carry the same complete decision
// set. Answer order is semantic because the command consumes the runtime's
// ordered interaction set atomically.
func (r ResumeRun) Equal(other ResumeRun) bool {
	if r.CommandID != other.CommandID || r.RunID != other.RunID || (r.Message == nil) != (other.Message == nil) {
		return false
	}
	if r.Message != nil && !r.Message.Equal(*other.Message) {
		return false
	}
	return slices.EqualFunc(r.Answers, other.Answers, func(left, right InterruptAnswer) bool {
		return left.ItemID == right.ItemID && AnswerEqual(left.Answer, right.Answer)
	})
}

// Clone detaches every mutable answer and optional message owned by a resume
// command. Delivery adapters may retain the clone across retries or process
// restarts without sharing the interaction editor's draft state.
func (r ResumeRun) Clone() ResumeRun {
	answers := r.Answers
	r.Answers = make([]InterruptAnswer, len(answers))
	for index, response := range answers {
		r.Answers[index] = InterruptAnswer{
			ItemID: response.ItemID,
			Answer: CloneAnswer(response.Answer),
		}
	}
	if r.Message != nil {
		message := r.Message.Clone()
		r.Message = &message
	}
	return r
}

type CancelRun struct {
	CommandID CommandID
	RunID     string
	Reason    string
}
