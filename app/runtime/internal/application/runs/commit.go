package runs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type ItemReplacement struct {
	Expected    transcript.Item
	Replacement transcript.Item
}

// WaitingSubtreeCancellationCommit is the immutable write-set for canceling a
// child while its Run tree is waiting.
type WaitingSubtreeCancellationCommit struct {
	RootRunID        string
	TargetRunID      string
	SessionID        string
	RootRun          transcript.Run
	ExpectedPending  Pending
	RemainingPending *Pending
	Checkpoint       ExecutorCheckpoint
	TerminalRuns     []transcript.Run
	TerminalItems    []ItemReplacement
	ParentItem       ItemReplacement
	Resume           *run.TreeResumeDraft
	OpeningEvents    []EventCommit
}

type WaitingSubtreeCancellationResult struct {
	TargetRun transcript.Run
	RootRun   transcript.Run
}

// OpeningCommit is the atomic acceptance write-set for one fresh admission or
// one continuation.
type OpeningCommit struct {
	Admit            *run.Draft
	Resume           *run.TreeResumeDraft
	ScheduledSession *session.Session
	SessionModel     *SessionModelUpdate
	ScheduleFiring   string
	Events           []EventCommit
}

// ResumeClaimCommit is the answer linearization write-set. Its transaction
// consumes the exact waiting hand-off and deletes the old checkpoint before an
// executor may be restored or signaled. A crash after this commit therefore
// has no recoverable pre-answer snapshot and boot reconciliation must mark the
// still-nonterminal tree lost.
type ResumeClaimCommit struct {
	Expected  Pending
	Answers   []InterruptAnswer
	ClaimedAt time.Time
}

// ClaimedResume is the immutable result of a successful answer claim. The
// checkpoint is returned from the same transaction that made it nonrecoverable;
// callers may hold it only long enough to stage the continuation in this use
// case and never persist it again.
type ClaimedResume struct {
	Pending    Pending
	Answers    []InterruptAnswer
	Checkpoint ExecutorCheckpoint
}

func (claim ResumeClaimCommit) Validate() error {
	if err := claim.Expected.Validate(); err != nil {
		return fmt.Errorf("runs: resume claim Pending: %w", err)
	}
	if claim.ClaimedAt.IsZero() {
		return errors.New("runs: resume claim time is required")
	}
	if len(claim.Answers) != len(claim.Expected.Bindings) {
		return fmt.Errorf(
			"runs: resume claim has %d answers for %d boundaries",
			len(claim.Answers), len(claim.Expected.Bindings),
		)
	}
	for index, answer := range claim.Answers {
		binding := claim.Expected.Bindings[index]
		if answer.InterruptItemID != binding.InterruptItemID || answer.MemberID != binding.MemberID ||
			answer.RequestID != binding.RequestID {
			return fmt.Errorf("runs: resume claim answer[%d] differs from its pending boundary", index)
		}
	}
	return nil
}

// SessionModelUpdate is committed with the Run admission that established it.
type SessionModelUpdate struct {
	SessionID string
	Model     string
}

type StateChange uint8

const (
	StateUnchanged StateChange = iota
	StateSuspend
	StateTerminalize
)

// ModelInvocationState records the durable application observation of one
// provider call. It is deliberately smaller than a model response: semantic
// output belongs to Transcript Items and accounting belongs to RunProgressCommit.
// This record exists to distinguish an invocation that never crossed the
// provider boundary from one whose final projection became indeterminate.
type ModelInvocationState uint8

const (
	ModelInvocationStarted ModelInvocationState = iota + 1
	ModelInvocationCompleted
	ModelInvocationFailed
	ModelInvocationUnknown
)

// ModelInvocationCommit is one monotonic transition in the durable invocation
// journal. StartedAt is repeated on terminal transitions so persistence can
// compare the exact attempt instead of updating whichever row happens to share
// CallID.
type ModelInvocationCommit struct {
	CallID     string
	SegmentID  string
	State      ModelInvocationState
	StartedAt  time.Time
	FinishedAt time.Time
}

// ToolInvocationState records whether one model-requested Tool call has only
// started, reached a definite result, or was closed without one at a Run
// boundary. Final Tool content still has exactly one owner: the Transcript Item
// committed beside the terminal transition.
type ToolInvocationState uint8

const (
	ToolInvocationStarted ToolInvocationState = iota + 1
	ToolInvocationCompleted
	ToolInvocationIncomplete
)

// ToolInvocationCommit is the durable pre-call/terminal attempt transition for
// one canonical Tool Item. ItemID connects the operational start boundary to
// the eventual Transcript projection without copying arguments or result data.
type ToolInvocationCommit struct {
	CallID     string
	ItemID     string
	SegmentID  string
	State      ToolInvocationState
	StartedAt  time.Time
	FinishedAt time.Time
}

func (commit ToolInvocationCommit) validate() error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: "call", value: commit.CallID},
		{name: "Item", value: commit.ItemID},
		{name: "segment", value: commit.SegmentID},
	} {
		name, value := identity.name, identity.value
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("runs: Tool invocation %s ID is required without surrounding whitespace", name)
		}
	}
	if commit.StartedAt.IsZero() {
		return errors.New("runs: Tool invocation start time is required")
	}
	switch commit.State {
	case ToolInvocationStarted:
		if !commit.FinishedAt.IsZero() {
			return errors.New("runs: started Tool invocation carries a finish time")
		}
	case ToolInvocationCompleted, ToolInvocationIncomplete:
		if commit.FinishedAt.IsZero() {
			return errors.New("runs: terminal Tool invocation has no finish time")
		}
		if commit.FinishedAt.Before(commit.StartedAt) {
			return errors.New("runs: Tool invocation finish time precedes start time")
		}
	default:
		return fmt.Errorf("runs: Tool invocation has unknown state %d", commit.State)
	}
	return nil
}

func (commit ModelInvocationCommit) validate() error {
	if strings.TrimSpace(commit.CallID) == "" || commit.CallID != strings.TrimSpace(commit.CallID) {
		return errors.New("runs: model invocation call ID is required without surrounding whitespace")
	}
	if strings.TrimSpace(commit.SegmentID) == "" || commit.SegmentID != strings.TrimSpace(commit.SegmentID) {
		return errors.New("runs: model invocation segment ID is required without surrounding whitespace")
	}
	if commit.StartedAt.IsZero() {
		return errors.New("runs: model invocation start time is required")
	}
	switch commit.State {
	case ModelInvocationStarted:
		if !commit.FinishedAt.IsZero() {
			return errors.New("runs: started model invocation carries a finish time")
		}
	case ModelInvocationCompleted, ModelInvocationFailed, ModelInvocationUnknown:
		if commit.FinishedAt.IsZero() {
			return errors.New("runs: terminal model invocation has no finish time")
		}
		if commit.FinishedAt.Before(commit.StartedAt) {
			return errors.New("runs: model invocation finish time precedes start time")
		}
	default:
		return fmt.Errorf("runs: model invocation has unknown state %d", commit.State)
	}
	return nil
}

// RunProgressCommit is the durable cumulative accounting snapshot produced at
// a model-response boundary. SegmentID fences the update to the exact running
// segment; a stale continuation cannot overwrite a newer Run.
type RunProgressCommit struct {
	SegmentID string
	Metrics   transcript.RunMetrics
	UpdatedAt time.Time
}

func (progress RunProgressCommit) validate() error {
	if strings.TrimSpace(progress.SegmentID) == "" || progress.SegmentID != strings.TrimSpace(progress.SegmentID) {
		return errors.New("runs: progress segment ID is required without surrounding whitespace")
	}
	if progress.UpdatedAt.IsZero() {
		return errors.New("runs: progress update time is required")
	}
	if err := progress.Metrics.Validate(); err != nil {
		return fmt.Errorf("runs: progress metrics: %w", err)
	}
	return nil
}

type EventCommit struct {
	RunID     string
	SessionID string
	State     StateChange
	Outcome   run.Outcome
	Items     []transcript.Item
	// ModelInvocations and Progress are application observations committed in
	// the same transaction as the semantic Transcript Items derived from one
	// authoritative executor fact.
	ModelInvocations []ModelInvocationCommit
	ToolInvocations  []ToolInvocationCommit
	Progress         *RunProgressCommit
	Run              *transcript.Run
	GoalRun          *goal.RunRecord
	// ObsoleteCheckpointRootID identifies the executor checkpoint aggregate the
	// root Run terminal makes obsolete. Child terminal commits leave it empty.
	ObsoleteCheckpointRootID string
}

// Validate proves that one event projection is owner-bound and that any Goal
// charge is exactly the accounting fact implied by its terminal Run.
func (c EventCommit) Validate() error {
	if err := c.validateEnvelope(); err != nil {
		return err
	}
	if err := c.validateItems(); err != nil {
		return err
	}
	if err := c.validateInvocations(); err != nil {
		return err
	}
	if c.Progress != nil {
		if err := c.Progress.validate(); err != nil {
			return err
		}
	}
	return c.validateLifecycle()
}

func (c EventCommit) validateEnvelope() error {
	if strings.TrimSpace(c.RunID) == "" || c.RunID != strings.TrimSpace(c.RunID) {
		return errors.New("runs: event commit Run ID must be non-empty without surrounding whitespace")
	}
	if strings.TrimSpace(c.SessionID) == "" || c.SessionID != strings.TrimSpace(c.SessionID) {
		return errors.New("runs: event commit Session ID must be non-empty without surrounding whitespace")
	}
	if c.ObsoleteCheckpointRootID != strings.TrimSpace(c.ObsoleteCheckpointRootID) {
		return errors.New("runs: event commit checkpoint root ID has surrounding whitespace")
	}
	return nil
}

func (c EventCommit) validateItems() error {
	seenItems := make(map[string]struct{}, len(c.Items))
	for index, item := range c.Items {
		if item.ID == "" || item.RunID != c.RunID || item.SessionID != c.SessionID {
			return fmt.Errorf("runs: event commit Item[%d] is not owned by Run %q", index, c.RunID)
		}
		if _, duplicate := seenItems[item.ID]; duplicate {
			return fmt.Errorf("runs: event commit repeats Item %q", item.ID)
		}
		seenItems[item.ID] = struct{}{}
		if err := item.Validate(); err != nil {
			return fmt.Errorf("runs: event commit Item %q: %w", item.ID, err)
		}
	}
	return nil
}

func (c EventCommit) validateInvocations() error {
	seenInvocations := make(map[string]struct{}, len(c.ModelInvocations))
	for index, invocation := range c.ModelInvocations {
		if err := invocation.validate(); err != nil {
			return fmt.Errorf("runs: event commit model invocation[%d]: %w", index, err)
		}
		if _, duplicate := seenInvocations[invocation.CallID]; duplicate {
			return fmt.Errorf("runs: event commit repeats model invocation %q", invocation.CallID)
		}
		seenInvocations[invocation.CallID] = struct{}{}
	}
	seenTools := make(map[string]struct{}, len(c.ToolInvocations))
	seenToolItems := make(map[string]struct{}, len(c.ToolInvocations))
	for index, invocation := range c.ToolInvocations {
		if err := invocation.validate(); err != nil {
			return fmt.Errorf("runs: event commit Tool invocation[%d]: %w", index, err)
		}
		if _, duplicate := seenTools[invocation.CallID]; duplicate {
			return fmt.Errorf("runs: event commit repeats Tool invocation %q", invocation.CallID)
		}
		if _, duplicate := seenToolItems[invocation.ItemID]; duplicate {
			return fmt.Errorf("runs: event commit repeats Tool invocation Item %q", invocation.ItemID)
		}
		seenTools[invocation.CallID] = struct{}{}
		seenToolItems[invocation.ItemID] = struct{}{}
	}
	return nil
}

func (c EventCommit) validateLifecycle() error {
	switch c.State {
	case StateUnchanged:
		if c.Run != nil || c.GoalRun != nil || c.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: unchanged event commit carries lifecycle facts")
		}
		return nil
	case StateSuspend:
		if c.Run == nil || c.Run.State != run.Waiting {
			return errors.New("runs: suspend event commit has no waiting Run")
		}
		if c.GoalRun != nil || c.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: suspend event commit carries terminal facts")
		}
	case StateTerminalize:
		if c.Run == nil || !c.Run.State.IsTerminal() || c.Run.Outcome == nil || *c.Run.Outcome != c.Outcome {
			return errors.New("runs: terminal event commit has no matching terminal Run")
		}
	default:
		return fmt.Errorf("runs: event commit has unknown state change %d", c.State)
	}

	if c.Run.ID != c.RunID || c.Run.SessionID != c.SessionID {
		return errors.New("runs: event commit Run ownership differs from its envelope")
	}
	validatedRun := *c.Run
	if c.State == StateTerminalize && validatedRun.MessageMark == transcript.UnknownMessageMark {
		// The reducer cannot know the final conversation watermark. The terminal
		// transaction resolves it while committing this Run; every other terminal
		// fact must already satisfy the domain invariant.
		validatedRun.MessageMark = 0
	}
	if err := validatedRun.Validate(); err != nil {
		return fmt.Errorf("runs: event commit Run: %w", err)
	}
	if c.State == StateSuspend {
		return nil
	}
	return validateTerminalGoalRun(*c.Run, c.GoalRun)
}

func validateTerminalGoalRun(run transcript.Run, record *goal.RunRecord) error {
	if run.GoalLeaseID == "" {
		if record != nil {
			return fmt.Errorf("runs: non-Goal Run %q carries a Goal Run", run.ID)
		}
		return nil
	}
	if !run.Lineage().IsRoot() {
		return fmt.Errorf("runs: child Run %q carries a root Goal lease", run.ID)
	}
	if record == nil {
		return fmt.Errorf("runs: Goal-owned terminal Run %q has no Goal Run", run.ID)
	}
	if err := record.Validate(); err != nil {
		return fmt.Errorf("runs: terminal Goal Run: %w", err)
	}
	costUSD := 0.0
	if run.Metrics.Usage != nil && run.Metrics.Usage.CostUSD != nil {
		costUSD = *run.Metrics.Usage.CostUSD
	}
	if run.Outcome == nil || record.SessionID != run.SessionID || record.LeaseID != run.GoalLeaseID ||
		record.RunID != run.ID || record.Outcome != *run.Outcome || record.CostUSD != costUSD ||
		record.Steps != run.Metrics.Steps || !record.CompletedAt.Equal(run.FinishedAt) {
		return fmt.Errorf("runs: Goal Run differs from terminal Run %q", run.ID)
	}
	return nil
}

func (c EventCommit) isEmpty() bool {
	return len(c.Items) == 0 &&
		len(c.ModelInvocations) == 0 &&
		len(c.ToolInvocations) == 0 &&
		c.Progress == nil &&
		c.Run == nil &&
		c.GoalRun == nil &&
		c.ObsoleteCheckpointRootID == "" &&
		c.State == StateUnchanged
}

// Validate proves that the opening is exactly one fresh admission or one tree
// continuation and that every accompanying projection is item-only. Persistence
// Port implementations may reject unavailable stores or concurrent state changes, but they
// do not reinterpret this application write-set.
func (c OpeningCommit) Validate() error {
	if (c.Admit == nil) == (c.Resume == nil) {
		return errors.New("runs: opening requires exactly one admission action")
	}
	if c.Admit != nil {
		if c.ScheduledSession != nil && c.ScheduledSession.ID != c.Admit.SessionID {
			return errors.New("runs: opening scheduled Session differs from admitted Run")
		}
		if c.SessionModel != nil {
			if c.SessionModel.SessionID != c.Admit.SessionID {
				return errors.New("runs: opening Session model differs from admitted Run")
			}
			if strings.TrimSpace(c.SessionModel.Model) == "" || c.SessionModel.Model != strings.TrimSpace(c.SessionModel.Model) {
				return errors.New("runs: opening Session model must be non-empty without surrounding whitespace")
			}
		}
		if c.ScheduleFiring != strings.TrimSpace(c.ScheduleFiring) {
			return errors.New("runs: opening schedule firing has surrounding whitespace")
		}
	} else if c.ScheduledSession != nil || c.SessionModel != nil || c.ScheduleFiring != "" {
		return errors.New("runs: resumed opening carries fresh-run facts")
	}
	for index, commit := range c.Events {
		if err := commit.Validate(); err != nil {
			return fmt.Errorf("runs: opening event[%d]: %w", index, err)
		}
		if commit.State != StateUnchanged || len(commit.Items) == 0 {
			return fmt.Errorf("runs: opening event[%d] is not an item-only projection", index)
		}
	}
	return nil
}

// TreeBarrierCommit is the one durable write-set produced when any executor
// interruption stops a Run tree. Pending owns the complete continuation hand-off;
// Runs contains one StateSuspend commit for every active Run in deterministic
// postorder. No individual Run commit may write or consume the root-owned set.
type TreeBarrierCommit struct {
	Pending    Pending
	Runs       []EventCommit
	Checkpoint ExecutorCheckpoint
}

// Validate proves that the barrier is the complete interruption projection for
// the pending continuation tree and that its checkpoint belongs to the same
// run. The Effects port only persists this already-defined write-set.
func (c TreeBarrierCommit) Validate() error {
	if err := c.Pending.Validate(); err != nil {
		return fmt.Errorf("runs: tree barrier Pending: %w", err)
	}
	rootContinuation, found := c.Pending.RootContinuation()
	if !found {
		return errors.New("runs: tree barrier has no root continuation")
	}
	validator := treeBarrierValidator{
		barrier:       c,
		continuations: make(map[string]Continuation, len(c.Pending.Continuations)),
		seenRunIDs:    make(map[string]struct{}, len(c.Runs)),
	}
	for _, continuation := range c.Pending.Continuations {
		validator.continuations[continuation.RunID] = continuation
	}
	if err := validator.validateCheckpoint(rootContinuation); err != nil {
		return err
	}
	return validator.validateRuns()
}

type treeBarrierValidator struct {
	barrier       TreeBarrierCommit
	continuations map[string]Continuation
	seenRunIDs    map[string]struct{}
}

func (validator treeBarrierValidator) validateCheckpoint(rootContinuation Continuation) error {
	checkpoint := validator.barrier.Checkpoint
	pending := validator.barrier.Pending
	if err := checkpoint.ValidateOwnership(rootContinuation.MemberID, pending.SessionID); err != nil {
		return fmt.Errorf("runs: tree barrier checkpoint ownership: %w", err)
	}
	if checkpoint.Scope.GoalLeaseID != pending.GoalLeaseID {
		return fmt.Errorf(
			"runs: tree barrier checkpoint goal lease %q does not match Pending %q: %w",
			checkpoint.Scope.GoalLeaseID,
			pending.GoalLeaseID,
			ErrInvalidExecutorCheckpoint,
		)
	}
	if checkpoint.ModelSelection != rootContinuation.ModelSelection {
		return fmt.Errorf("runs: tree barrier checkpoint model differs from root continuation: %w", ErrInvalidExecutorCheckpoint)
	}
	if checkpoint.Limits != rootContinuation.Limits {
		return fmt.Errorf("runs: tree barrier checkpoint limits differ from root continuation: %w", ErrInvalidExecutorCheckpoint)
	}
	return nil
}

func (validator treeBarrierValidator) validateRuns() error {
	if len(validator.barrier.Runs) != len(validator.barrier.Pending.Continuations) {
		return fmt.Errorf(
			"runs: tree barrier has %d Run commits for %d continuations",
			len(validator.barrier.Runs),
			len(validator.barrier.Pending.Continuations),
		)
	}
	for index, runCommit := range validator.barrier.Runs {
		if err := validator.validateRun(index, runCommit); err != nil {
			return err
		}
	}
	return nil
}

func (validator treeBarrierValidator) validateRun(index int, runCommit EventCommit) error {
	if err := runCommit.Validate(); err != nil {
		return fmt.Errorf("runs: tree barrier Run[%d]: %w", index, err)
	}
	if runCommit.State != StateSuspend || runCommit.Run == nil || runCommit.Run.State != run.Waiting {
		return fmt.Errorf("runs: tree barrier Run[%d] is not a waiting Run projection", index)
	}
	pending := validator.barrier.Pending
	if runCommit.SessionID != pending.SessionID || runCommit.Run.SessionID != pending.SessionID {
		return fmt.Errorf("runs: tree barrier Run[%d] Session differs from Pending", index)
	}
	continuation, exists := validator.continuations[runCommit.RunID]
	if !exists {
		return fmt.Errorf("runs: tree barrier Run[%d] has no continuation", index)
	}
	if runCommit.Run.Lineage() != continuation.Lineage ||
		runCommit.Run.ModelSelection != continuation.ModelSelection ||
		!runCommit.Run.CreatedAt.Equal(continuation.RunCreatedAt) ||
		!runCommit.Run.Metrics.Equal(continuation.Metrics) ||
		runCommit.Run.Limits != continuation.Limits {
		return fmt.Errorf("runs: tree barrier Run[%d] differs from its continuation", index)
	}
	if !runCommit.Run.Capabilities.Equal(pending.Capabilities) {
		return fmt.Errorf("runs: tree barrier Run[%d] capabilities differ from Pending", index)
	}
	if runCommit.RunID == pending.RootRunID {
		if runCommit.Run.GoalLeaseID != pending.GoalLeaseID {
			return errors.New("runs: tree barrier root Run goal lease differs from Pending")
		}
	} else if runCommit.Run.GoalLeaseID != "" {
		return fmt.Errorf("runs: tree barrier child Run[%d] carries a root Goal lease", index)
	}
	if _, duplicate := validator.seenRunIDs[runCommit.RunID]; duplicate {
		return fmt.Errorf("runs: tree barrier repeats Run %q", runCommit.RunID)
	}
	validator.seenRunIDs[runCommit.RunID] = struct{}{}
	return nil
}
