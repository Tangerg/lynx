package run

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
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
func (run Run) Snapshot() Snapshot {
	return Snapshot{
		SessionID: run.sessionID, ID: run.id, Lineage: run.lineage,
		ModelSelection: run.modelSelection, GoalIncarnationID: run.goalIncarnationID,
		State: run.state, ActiveSegmentID: run.activeSegmentID,
		Outcome: cloneOutcome(run.outcome), Detail: run.detail,
		Failure: cloneFailure(run.failure), Metrics: run.metrics,
		ContextTokens: run.contextTokens,
		Limits:        run.limits, Capabilities: run.capabilities.Clone(),
		CreatedAt: run.createdAt, FinishedAt: run.finishedAt,
		UpdatedAt: run.updatedAt, MessageMark: run.messageMark,
	}
}

// Fork derives a terminal historical Run for a child Session under fresh
// identities. It preserves the completed work facts, replaces root/child
// lineage with the caller-resolved equivalent, and clears Goal attribution
// because a Session branch does not inherit the parent's Goal incarnation.
func (run Run) Fork(sessionID, id string, lineage Lineage) (Run, error) {
	if !run.state.IsTerminal() {
		return Run{}, errors.New("run: only a terminal Run can be forked")
	}
	if run.lineage.IsRoot() != lineage.IsRoot() {
		return Run{}, errors.New("run: fork changes root/child lineage kind")
	}
	snapshot := run.Snapshot()
	snapshot.SessionID = sessionID
	snapshot.ID = id
	snapshot.Lineage = lineage
	snapshot.GoalIncarnationID = ""
	return Restore(snapshot)
}

// Equal reports whether two values contain the same authoritative Run facts.
// It is useful at persistence and hand-off boundaries that must prove a
// proposed transition was derived from the currently committed aggregate.
func (run Run) Equal(other Run) bool {
	if run.sessionID != other.sessionID || run.id != other.id || run.lineage != other.lineage ||
		run.modelSelection != other.modelSelection || run.goalIncarnationID != other.goalIncarnationID ||
		run.state != other.state || run.activeSegmentID != other.activeSegmentID ||
		run.detail != other.detail || !run.metrics.Equal(other.metrics) ||
		run.contextTokens != other.contextTokens || run.limits != other.limits ||
		!run.capabilities.Equal(other.capabilities) || !run.createdAt.Equal(other.createdAt) ||
		!run.finishedAt.Equal(other.finishedAt) || !run.updatedAt.Equal(other.updatedAt) ||
		run.messageMark != other.messageMark {
		return false
	}
	if run.outcome == nil || other.outcome == nil {
		if run.outcome != nil || other.outcome != nil {
			return false
		}
	} else if *run.outcome != *other.outcome {
		return false
	}
	if run.failure == nil || other.failure == nil {
		return run.failure == nil && other.failure == nil
	}
	return *run.failure == *other.failure
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
func (run Run) Validate() error {
	switch {
	case strings.TrimSpace(run.id) == "" || run.id != strings.TrimSpace(run.id):
		return errors.New("run: ID is required without surrounding whitespace")
	case strings.TrimSpace(run.sessionID) == "" || run.sessionID != strings.TrimSpace(run.sessionID):
		return errors.New("run: Session ID is required without surrounding whitespace")
	case run.createdAt.IsZero():
		return errors.New("run: creation time is required")
	case run.updatedAt.IsZero():
		return errors.New("run: update time is required")
	case run.updatedAt.Before(run.createdAt):
		return errors.New("run: update time precedes creation")
	}
	if err := run.lineage.Validate(run.id); err != nil {
		return err
	}
	if err := run.modelSelection.Validate(); err != nil {
		return fmt.Errorf("run: model selection: %w", err)
	}
	if run.goalIncarnationID != strings.TrimSpace(run.goalIncarnationID) {
		return errors.New("run: goal incarnation ID has surrounding whitespace")
	}
	if run.lineage.IsChild() && run.goalIncarnationID != "" {
		return errors.New("run: child carries a root Goal incarnation")
	}
	if (run.state == Running) != (run.activeSegmentID != "") {
		return fmt.Errorf("run: %s Run has active Segment %q", run.state, run.activeSegmentID)
	}
	if err := run.metrics.Validate(); err != nil {
		return err
	}
	if run.contextTokens < 0 {
		return errors.New("run: context tokens must not be negative")
	}
	if err := run.limits.Validate(); err != nil {
		return err
	}
	if err := run.capabilities.Validate(); err != nil {
		return err
	}
	if run.state.IsTerminal() {
		return run.validateTerminal()
	}
	return run.validateOpen()
}

func (run Run) validateOpen() error {
	switch {
	case run.outcome != nil:
		return fmt.Errorf("run: %s Run carries an outcome", run.state)
	case run.failure != nil:
		return fmt.Errorf("run: %s Run carries a failure", run.state)
	case run.detail != "":
		return fmt.Errorf("run: %s Run carries terminal detail", run.state)
	case !run.finishedAt.IsZero():
		return fmt.Errorf("run: %s Run carries finish time", run.state)
	case run.messageMark != UnknownMessageMark:
		return fmt.Errorf("run: %s Run carries message watermark %d", run.state, run.messageMark)
	}
	return nil
}

func (run Run) validateTerminal() error {
	if run.outcome == nil {
		return errors.New("run: terminal Run has no outcome")
	}
	expected, ok := Running.Terminate(*run.outcome)
	if !ok || expected != run.state {
		return fmt.Errorf("run: state %s does not match outcome %s", run.state, run.outcome)
	}
	switch *run.outcome {
	case OutcomeFailed:
		if run.failure == nil {
			return errors.New("run: failed Run has no failure")
		}
	case OutcomeTimedOut:
		if run.failure == nil || run.failure.Kind != FailureTimeout {
			return errors.New("run: timed-out Run has no timeout failure")
		}
	case OutcomeLost:
		if run.failure == nil || run.failure.Kind != FailureLost {
			return errors.New("run: lost Run has no lost failure")
		}
	default:
		if run.failure != nil {
			return fmt.Errorf("run: outcome %s carries a failure", run.outcome)
		}
	}
	if run.failure != nil {
		if err := run.failure.Validate(); err != nil {
			return err
		}
	}
	switch {
	case run.finishedAt.IsZero():
		return errors.New("run: terminal Run has no finish time")
	case run.finishedAt.Before(run.createdAt):
		return errors.New("run: finish time precedes creation")
	case run.updatedAt.Before(run.finishedAt):
		return errors.New("run: update time precedes finish")
	case run.messageMark < UnknownMessageMark:
		return fmt.Errorf("run: terminal Run has message watermark %d", run.messageMark)
	}
	return nil
}

// AdvanceProgress returns a Run with one model-response boundary committed.
// Metrics are cumulative and must remain monotonic. contextTokens is a latest
// point-in-time prompt footprint and may decrease after compaction; zero means
// the provider supplied no authoritative footprint, so the prior value remains.
func (run Run) AdvanceProgress(metrics Metrics, contextTokens int64, updatedAt time.Time) (Run, error) {
	if run.state.IsTerminal() {
		return Run{}, errors.New("run: terminal Run cannot advance progress")
	}
	if err := metrics.ValidateAdvanceFrom(run.metrics); err != nil {
		return Run{}, err
	}
	if contextTokens < 0 {
		return Run{}, errors.New("run: context tokens must not be negative")
	}
	if err := run.validateTransitionTime(updatedAt); err != nil {
		return Run{}, err
	}
	run.metrics = metrics
	if contextTokens > 0 {
		run.contextTokens = contextTokens
	}
	run.updatedAt = updatedAt.UTC()
	return run, nil
}

// Suspend parks a Running Run and clears its active Segment.
func (run Run) Suspend(updatedAt time.Time) (Run, error) {
	next, ok := run.state.Suspend()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot suspend %s Run", run.state)
	}
	if err := run.validateTransitionTime(updatedAt); err != nil {
		return Run{}, err
	}
	run.state, run.activeSegmentID, run.updatedAt = next, "", updatedAt.UTC()
	return run, nil
}

// Resume opens a fresh Segment for a Waiting Run.
func (run Run) Resume(segmentID string, resumedAt time.Time) (Run, error) {
	next, ok := run.state.Resume()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot resume %s Run", run.state)
	}
	if strings.TrimSpace(segmentID) == "" || segmentID != strings.TrimSpace(segmentID) {
		return Run{}, errors.New("run: continuation Segment ID is required without surrounding whitespace")
	}
	if err := run.validateTransitionTime(resumedAt); err != nil {
		return Run{}, err
	}
	run.state, run.activeSegmentID, run.updatedAt = next, segmentID, resumedAt.UTC()
	return run, nil
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
func (run Run) Terminate(termination Termination) (Run, error) {
	next, ok := run.state.Terminate(termination.Outcome)
	if !ok {
		return Run{}, fmt.Errorf("run: outcome %s cannot terminate %s Run", termination.Outcome, run.state)
	}
	return run.finish(next, termination)
}

// CancelWaiting finishes a Waiting Run as canceled.
func (run Run) CancelWaiting(detail string, finishedAt time.Time, messageMark int) (Run, error) {
	if run.state != Waiting {
		return Run{}, fmt.Errorf("run: cannot cancel %s Run as waiting", run.state)
	}
	return run.finish(Canceled, Termination{Outcome: OutcomeCanceled, Detail: detail, FinishedAt: finishedAt, MessageMark: messageMark})
}

// RecoverLost finishes a non-terminal Run whose executor state cannot be recovered.
func (run Run) RecoverLost(failure Failure, finishedAt time.Time, messageMark int) (Run, error) {
	next, ok := run.state.RecoverLost()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot recover terminal %s Run as lost", run.state)
	}
	if failure.Kind != FailureLost {
		return Run{}, errors.New("run: lost recovery requires a lost failure")
	}
	return run.finish(next, Termination{Outcome: OutcomeLost, Failure: &failure, FinishedAt: finishedAt, MessageMark: messageMark})
}

func (run Run) finish(state State, termination Termination) (Run, error) {
	if err := run.validateTransitionTime(termination.FinishedAt); err != nil {
		return Run{}, err
	}
	run.state, run.activeSegmentID = state, ""
	run.outcome = cloneOutcome(&termination.Outcome)
	run.detail, run.failure = termination.Detail, cloneFailure(termination.Failure)
	run.finishedAt, run.updatedAt = termination.FinishedAt.UTC(), termination.FinishedAt.UTC()
	run.messageMark = termination.MessageMark
	if err := run.Validate(); err != nil {
		return Run{}, err
	}
	return run, nil
}

// WithMessageMark replaces a terminal Run's conversation coordinate without
// changing any other fact. Terminalization uses it after obtaining the final
// count; conversation compaction uses it to rebase an already-final boundary
// into the replacement history's coordinate space.
func (run Run) WithMessageMark(messageMark int) (Run, error) {
	if !run.state.IsTerminal() {
		return Run{}, errors.New("run: only terminal Run can resolve message watermark")
	}
	if messageMark < 0 {
		return Run{}, errors.New("run: message watermark must not be negative")
	}
	run.messageMark = messageMark
	if err := run.Validate(); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (run Run) validateTransitionTime(at time.Time) error {
	if at.IsZero() {
		return errors.New("run: transition time is required")
	}
	at = at.UTC()
	if at.Before(run.updatedAt) {
		return errors.New("run: transition time precedes last update")
	}
	return nil
}

func (run Run) ID() string                         { return run.id }
func (run Run) SessionID() string                  { return run.sessionID }
func (run Run) Lineage() Lineage                   { return run.lineage }
func (run Run) ModelSelection() modelref.Selection { return run.modelSelection }
func (run Run) GoalIncarnationID() string          { return run.goalIncarnationID }
func (run Run) State() State                       { return run.state }
func (run Run) ActiveSegmentID() string            { return run.activeSegmentID }
func (run Run) Outcome() (Outcome, bool) {
	if run.outcome == nil {
		return "", false
	}
	return *run.outcome, true
}
func (run Run) Detail() string { return run.detail }
func (run Run) Failure() (Failure, bool) {
	if run.failure == nil {
		return Failure{}, false
	}
	return *cloneFailure(run.failure), true
}
func (run Run) Metrics() Metrics           { return run.metrics }
func (run Run) ContextTokens() int64       { return run.contextTokens }
func (run Run) Limits() Limits             { return run.limits }
func (run Run) Capabilities() Capabilities { return run.capabilities.Clone() }
func (run Run) CreatedAt() time.Time       { return run.createdAt }
func (run Run) FinishedAt() time.Time      { return run.finishedAt }
func (run Run) UpdatedAt() time.Time       { return run.updatedAt }
func (run Run) MessageMark() int           { return run.messageMark }
