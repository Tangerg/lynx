package run

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
)

// ErrIdentityConflict reports an attempt to reuse a durable Run identity for a
// different Session.
var ErrIdentityConflict = errors.New("run: identity conflict")

// UnknownMessageMark is the watermark of a Run whose final conversation count
// is not known yet. It cannot be confused with a real count, including zero.
const UnknownMessageMark = -1

// Run is one logical unit of agent work from admission through any waiting and
// resume boundaries to exactly one terminal outcome. Its fields are private so
// every lifecycle and accounting change crosses a validated domain behavior.
type Run struct {
	sessionID         string
	id                string
	lineage           Lineage
	modelSelection    modelref.Selection
	goalIncarnationID string
	state             State
	activeSegmentID   string
	outcome           *Outcome
	detail            string
	failure           *Failure
	metrics           Metrics
	contextTokens     int64
	limits            Limits
	capabilities      Capabilities
	createdAt         time.Time
	finishedAt        time.Time
	updatedAt         time.Time
	messageMark       int
}

// Snapshot is the complete immutable value needed to restore or persist a Run.
// Restore validates every field; it is not a second mutation surface.
type Snapshot struct {
	SessionID         string
	ID                string
	Lineage           Lineage
	ModelSelection    modelref.Selection
	GoalIncarnationID string
	State             State
	ActiveSegmentID   string
	Outcome           *Outcome
	Detail            string
	Failure           *Failure
	Metrics           Metrics
	// ContextTokens is the latest completed model request's prompt footprint.
	// Zero means no authoritative footprint has been observed yet.
	ContextTokens int64
	Limits        Limits
	Capabilities  Capabilities
	CreatedAt     time.Time
	FinishedAt    time.Time
	UpdatedAt     time.Time
	MessageMark   int
}

// Admit creates the authoritative aggregate for a fresh root or child Run.
func Admit(draft Draft) (Run, error) {
	if err := draft.Validate(); err != nil {
		return Run{}, err
	}
	return Restore(Snapshot{
		SessionID: draft.SessionID, ID: draft.RunID, Lineage: draft.Lineage(),
		ModelSelection: draft.ModelSelection, GoalIncarnationID: draft.GoalIncarnationID,
		State: Running, ActiveSegmentID: draft.SegmentID, Limits: draft.Limits,
		Capabilities: draft.Capabilities, CreatedAt: draft.CreatedAt.UTC(),
		UpdatedAt: draft.CreatedAt.UTC(), MessageMark: UnknownMessageMark,
	})
}

// Restore rebuilds a Run from durable values and rejects an invalid snapshot.
func Restore(snapshot Snapshot) (Run, error) {
	run := Run{
		sessionID: snapshot.SessionID, id: snapshot.ID, lineage: snapshot.Lineage,
		modelSelection: snapshot.ModelSelection, goalIncarnationID: snapshot.GoalIncarnationID,
		state: snapshot.State, activeSegmentID: snapshot.ActiveSegmentID,
		outcome: cloneOutcome(snapshot.Outcome), detail: snapshot.Detail,
		failure: cloneFailure(snapshot.Failure), metrics: snapshot.Metrics,
		contextTokens: snapshot.ContextTokens,
		limits:        snapshot.Limits, capabilities: snapshot.Capabilities.Clone(),
		createdAt: snapshot.CreatedAt.UTC(), finishedAt: snapshot.FinishedAt.UTC(),
		updatedAt: snapshot.UpdatedAt.UTC(), messageMark: snapshot.MessageMark,
	}
	if err := run.Validate(); err != nil {
		return Run{}, err
	}
	return run, nil
}

// Snapshot returns a complete ownership-isolated value.
func (r Run) Snapshot() Snapshot {
	return Snapshot{
		SessionID: r.sessionID, ID: r.id, Lineage: r.lineage,
		ModelSelection: r.modelSelection, GoalIncarnationID: r.goalIncarnationID,
		State: r.state, ActiveSegmentID: r.activeSegmentID,
		Outcome: cloneOutcome(r.outcome), Detail: r.detail,
		Failure: cloneFailure(r.failure), Metrics: r.metrics,
		ContextTokens: r.contextTokens,
		Limits:        r.limits, Capabilities: r.capabilities.Clone(),
		CreatedAt: r.createdAt, FinishedAt: r.finishedAt,
		UpdatedAt: r.updatedAt, MessageMark: r.messageMark,
	}
}

// Fork derives a terminal historical Run for a child Session under fresh
// identities. It preserves the completed work facts, replaces root/child
// lineage with the caller-resolved equivalent, and clears Goal attribution
// because a Session branch does not inherit the parent's Goal incarnation.
func (r Run) Fork(sessionID, id string, lineage Lineage) (Run, error) {
	if !r.state.IsTerminal() {
		return Run{}, errors.New("run: only a terminal Run can be forked")
	}
	if r.lineage.IsRoot() != lineage.IsRoot() {
		return Run{}, errors.New("run: fork changes root/child lineage kind")
	}
	snapshot := r.Snapshot()
	snapshot.SessionID = sessionID
	snapshot.ID = id
	snapshot.Lineage = lineage
	snapshot.GoalIncarnationID = ""
	return Restore(snapshot)
}

// Equal reports whether two values contain the same authoritative Run facts.
// It is useful at persistence and hand-off boundaries that must prove a
// proposed transition was derived from the currently committed aggregate.
func (r Run) Equal(other Run) bool {
	if r.sessionID != other.sessionID || r.id != other.id || r.lineage != other.lineage ||
		r.modelSelection != other.modelSelection || r.goalIncarnationID != other.goalIncarnationID ||
		r.state != other.state || r.activeSegmentID != other.activeSegmentID ||
		r.detail != other.detail || !r.metrics.Equal(other.metrics) ||
		r.contextTokens != other.contextTokens || r.limits != other.limits ||
		!r.capabilities.Equal(other.capabilities) || !r.createdAt.Equal(other.createdAt) ||
		!r.finishedAt.Equal(other.finishedAt) || !r.updatedAt.Equal(other.updatedAt) ||
		r.messageMark != other.messageMark {
		return false
	}
	if r.outcome == nil || other.outcome == nil {
		if r.outcome != nil || other.outcome != nil {
			return false
		}
	} else if *r.outcome != *other.outcome {
		return false
	}
	if r.failure == nil || other.failure == nil {
		return r.failure == nil && other.failure == nil
	}
	return *r.failure == *other.failure
}

func cloneOutcome(outcome *Outcome) *Outcome {
	if outcome == nil {
		return nil
	}
	copy := *outcome
	return &copy
}

func cloneFailure(failure *Failure) *Failure {
	if failure == nil {
		return nil
	}
	copy := *failure
	return &copy
}

// Validate reports whether all lifecycle, identity, accounting, and terminal
// facts agree.
func (r Run) Validate() error {
	switch {
	case strings.TrimSpace(r.id) == "" || r.id != strings.TrimSpace(r.id):
		return errors.New("run: ID is required without surrounding whitespace")
	case strings.TrimSpace(r.sessionID) == "" || r.sessionID != strings.TrimSpace(r.sessionID):
		return errors.New("run: Session ID is required without surrounding whitespace")
	case r.createdAt.IsZero():
		return errors.New("run: creation time is required")
	case r.updatedAt.IsZero():
		return errors.New("run: update time is required")
	case r.updatedAt.Before(r.createdAt):
		return errors.New("run: update time precedes creation")
	}
	if err := r.lineage.Validate(r.id); err != nil {
		return err
	}
	if err := r.modelSelection.Validate(); err != nil {
		return fmt.Errorf("run: model selection: %w", err)
	}
	if r.goalIncarnationID != strings.TrimSpace(r.goalIncarnationID) {
		return errors.New("run: goal incarnation ID has surrounding whitespace")
	}
	if r.lineage.IsChild() && r.goalIncarnationID != "" {
		return errors.New("run: child carries a root Goal incarnation")
	}
	if (r.state == Running) != (r.activeSegmentID != "") {
		return fmt.Errorf("run: %s Run has active Segment %q", r.state, r.activeSegmentID)
	}
	if err := r.metrics.Validate(); err != nil {
		return err
	}
	if r.contextTokens < 0 {
		return errors.New("run: context tokens must not be negative")
	}
	if err := r.limits.Validate(); err != nil {
		return err
	}
	if err := r.capabilities.Validate(); err != nil {
		return err
	}
	if r.state.IsTerminal() {
		return r.validateTerminal()
	}
	return r.validateOpen()
}

func (r Run) validateOpen() error {
	switch {
	case r.outcome != nil:
		return fmt.Errorf("run: %s Run carries an outcome", r.state)
	case r.failure != nil:
		return fmt.Errorf("run: %s Run carries a failure", r.state)
	case r.detail != "":
		return fmt.Errorf("run: %s Run carries terminal detail", r.state)
	case !r.finishedAt.IsZero():
		return fmt.Errorf("run: %s Run carries finish time", r.state)
	case r.messageMark != UnknownMessageMark:
		return fmt.Errorf("run: %s Run carries message watermark %d", r.state, r.messageMark)
	}
	return nil
}

func (r Run) validateTerminal() error {
	if r.outcome == nil {
		return errors.New("run: terminal Run has no outcome")
	}
	expected, ok := Running.Terminate(*r.outcome)
	if !ok || expected != r.state {
		return fmt.Errorf("run: state %s does not match outcome %s", r.state, r.outcome)
	}
	switch *r.outcome {
	case OutcomeFailed:
		if r.failure == nil {
			return errors.New("run: failed Run has no failure")
		}
	case OutcomeTimedOut:
		if r.failure == nil || r.failure.Kind != FailureTimeout {
			return errors.New("run: timed-out Run has no timeout failure")
		}
	case OutcomeLost:
		if r.failure == nil || r.failure.Kind != FailureLost {
			return errors.New("run: lost Run has no lost failure")
		}
	default:
		if r.failure != nil {
			return fmt.Errorf("run: outcome %s carries a failure", r.outcome)
		}
	}
	if r.failure != nil {
		if err := r.failure.Validate(); err != nil {
			return err
		}
	}
	switch {
	case r.finishedAt.IsZero():
		return errors.New("run: terminal Run has no finish time")
	case r.finishedAt.Before(r.createdAt):
		return errors.New("run: finish time precedes creation")
	case r.updatedAt.Before(r.finishedAt):
		return errors.New("run: update time precedes finish")
	case r.messageMark < UnknownMessageMark:
		return fmt.Errorf("run: terminal Run has message watermark %d", r.messageMark)
	}
	return nil
}

// AdvanceProgress returns a Run with one model-response boundary committed.
// Metrics are cumulative and must remain monotonic. contextTokens is a latest
// point-in-time prompt footprint and may decrease after compaction; zero means
// the provider supplied no authoritative footprint, so the prior value remains.
func (r Run) AdvanceProgress(metrics Metrics, contextTokens int64, updatedAt time.Time) (Run, error) {
	if r.state.IsTerminal() {
		return Run{}, errors.New("run: terminal Run cannot advance progress")
	}
	if err := metrics.ValidateAdvanceFrom(r.metrics); err != nil {
		return Run{}, err
	}
	if contextTokens < 0 {
		return Run{}, errors.New("run: context tokens must not be negative")
	}
	if err := r.validateTransitionTime(updatedAt); err != nil {
		return Run{}, err
	}
	r.metrics = metrics
	if contextTokens > 0 {
		r.contextTokens = contextTokens
	}
	r.updatedAt = updatedAt.UTC()
	return r, nil
}

// Suspend parks a Running Run and clears its active Segment.
func (r Run) Suspend(updatedAt time.Time) (Run, error) {
	next, ok := r.state.Suspend()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot suspend %s Run", r.state)
	}
	if err := r.validateTransitionTime(updatedAt); err != nil {
		return Run{}, err
	}
	r.state, r.activeSegmentID, r.updatedAt = next, "", updatedAt.UTC()
	return r, nil
}

// Resume opens a fresh Segment for a Waiting Run.
func (r Run) Resume(segmentID string, resumedAt time.Time) (Run, error) {
	next, ok := r.state.Resume()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot resume %s Run", r.state)
	}
	if strings.TrimSpace(segmentID) == "" || segmentID != strings.TrimSpace(segmentID) {
		return Run{}, errors.New("run: continuation Segment ID is required without surrounding whitespace")
	}
	if err := r.validateTransitionTime(resumedAt); err != nil {
		return Run{}, err
	}
	r.state, r.activeSegmentID, r.updatedAt = next, segmentID, resumedAt.UTC()
	return r, nil
}

// Termination is the complete fact set required to finish a Run.
type Termination struct {
	Outcome     Outcome
	Detail      string
	Failure     *Failure
	FinishedAt  time.Time
	MessageMark int
}

// Terminate finishes a Running Run with one coherent terminal fact set.
func (r Run) Terminate(termination Termination) (Run, error) {
	next, ok := r.state.Terminate(termination.Outcome)
	if !ok {
		return Run{}, fmt.Errorf("run: outcome %s cannot terminate %s Run", termination.Outcome, r.state)
	}
	return r.finish(next, termination)
}

// CancelWaiting finishes a Waiting Run as canceled.
func (r Run) CancelWaiting(detail string, finishedAt time.Time, messageMark int) (Run, error) {
	if r.state != Waiting {
		return Run{}, fmt.Errorf("run: cannot cancel %s Run as waiting", r.state)
	}
	return r.finish(Canceled, Termination{Outcome: OutcomeCanceled, Detail: detail, FinishedAt: finishedAt, MessageMark: messageMark})
}

// RecoverLost finishes a non-terminal Run whose executor state cannot be recovered.
func (r Run) RecoverLost(failure Failure, finishedAt time.Time, messageMark int) (Run, error) {
	next, ok := r.state.RecoverLost()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot recover terminal %s Run as lost", r.state)
	}
	if failure.Kind != FailureLost {
		return Run{}, errors.New("run: lost recovery requires a lost failure")
	}
	return r.finish(next, Termination{Outcome: OutcomeLost, Failure: &failure, FinishedAt: finishedAt, MessageMark: messageMark})
}

func (r Run) finish(state State, termination Termination) (Run, error) {
	if err := r.validateTransitionTime(termination.FinishedAt); err != nil {
		return Run{}, err
	}
	r.state, r.activeSegmentID = state, ""
	r.outcome = cloneOutcome(&termination.Outcome)
	r.detail, r.failure = termination.Detail, cloneFailure(termination.Failure)
	r.finishedAt, r.updatedAt = termination.FinishedAt.UTC(), termination.FinishedAt.UTC()
	r.messageMark = termination.MessageMark
	if err := r.Validate(); err != nil {
		return Run{}, err
	}
	return r, nil
}

// WithMessageMark replaces a terminal Run's conversation coordinate without
// changing any other fact. Terminalization uses it after obtaining the final
// count; conversation compaction uses it to rebase an already-final boundary
// into the replacement history's coordinate space.
func (r Run) WithMessageMark(messageMark int) (Run, error) {
	if !r.state.IsTerminal() {
		return Run{}, errors.New("run: only terminal Run can resolve message watermark")
	}
	if messageMark < 0 {
		return Run{}, errors.New("run: message watermark must not be negative")
	}
	r.messageMark = messageMark
	if err := r.Validate(); err != nil {
		return Run{}, err
	}
	return r, nil
}

func (r Run) validateTransitionTime(at time.Time) error {
	if at.IsZero() {
		return errors.New("run: transition time is required")
	}
	at = at.UTC()
	if at.Before(r.updatedAt) {
		return errors.New("run: transition time precedes last update")
	}
	return nil
}

func (r Run) ID() string                         { return r.id }
func (r Run) SessionID() string                  { return r.sessionID }
func (r Run) Lineage() Lineage                   { return r.lineage }
func (r Run) ModelSelection() modelref.Selection { return r.modelSelection }
func (r Run) GoalIncarnationID() string          { return r.goalIncarnationID }
func (r Run) State() State                       { return r.state }
func (r Run) ActiveSegmentID() string            { return r.activeSegmentID }
func (r Run) Outcome() (Outcome, bool) {
	if r.outcome == nil {
		return "", false
	}
	return *r.outcome, true
}
func (r Run) Detail() string { return r.detail }
func (r Run) Failure() (Failure, bool) {
	if r.failure == nil {
		return Failure{}, false
	}
	return *cloneFailure(r.failure), true
}
func (r Run) Metrics() Metrics           { return r.metrics }
func (r Run) ContextTokens() int64       { return r.contextTokens }
func (r Run) Limits() Limits             { return r.limits }
func (r Run) Capabilities() Capabilities { return r.capabilities.Clone() }
func (r Run) CreatedAt() time.Time       { return r.createdAt }
func (r Run) FinishedAt() time.Time      { return r.finishedAt }
func (r Run) UpdatedAt() time.Time       { return r.updatedAt }
func (r Run) MessageMark() int           { return r.messageMark }
