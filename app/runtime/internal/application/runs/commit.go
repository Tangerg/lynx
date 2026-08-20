package runs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type ItemReplacement struct {
	Expected    transcript.Item
	Replacement transcript.Item
}

// WaitingSubtreeCancellationCommit is the immutable write-set for canceling a
// child while its Run tree is waiting.
type WaitingSubtreeCancellationCommit struct {
	// CommitID identifies the complete cancellation transaction. A cancellation
	// that resumes the surviving tree reuses its OpeningCommit identity.
	CommitID             string
	RootRunID            string
	TargetRunID          string
	SessionID            string
	RootRun              run.Run
	ExpectedPending      Pending
	RemainingPending     *Pending
	Checkpoint           ExecutorCheckpoint
	TerminalRuns         []run.Run
	TerminalItems        []ItemReplacement
	ParentItem           ItemReplacement
	ConversationMessages []corechat.Message
	Resume               *run.TreeResumeDraft
	OpeningEvents        []EventCommit
}

type WaitingSubtreeCancellationResult struct {
	TargetRun run.Run
	RootRun   run.Run
}

// OpeningCommit is the atomic acceptance write-set for one fresh admission or
// one continuation.
type OpeningCommit struct {
	// CommitID identifies the complete admission/resume transaction, including
	// every opening projection. It is not an EventCommit identity.
	CommitID           string
	Admit              *run.Draft
	Resume             *run.TreeResumeDraft
	InitialSession     *session.Session
	SessionReplacement *SessionReplacement
	ScheduleFiring     string
	Events             []EventCommit
}

// ResumeClaimCommit is the answer linearization write-set. Its transaction
// consumes the exact waiting hand-off and deletes the old checkpoint before an
// executor may be restored or signaled. A crash after this commit therefore
// has no recoverable pre-answer snapshot and boot reconciliation must mark the
// still-nonterminal tree lost.
type ResumeClaimCommit struct {
	// CommitID identifies the complete answer-claim transaction. The checkpoint
	// returned by a successful claim remains a one-shot in-memory hand-off.
	CommitID  string
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

// ToolApprovalResolution is the exact durable ToolCall fact accepted by one
// human answer. The persistence boundary resolves this identity inside the same
// transaction that consumes the Pending barrier.
type ToolApprovalResolution struct {
	Identity   transcript.ItemIdentity
	CallID     string
	Invocation transcript.ToolInvocation
	Decision   approval.Decision
}

func (resolution ToolApprovalResolution) Validate() error {
	if err := resolution.Identity.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(resolution.CallID) == "" || resolution.CallID != strings.TrimSpace(resolution.CallID) {
		return errors.New("runs: approval Tool call identity is required without surrounding whitespace")
	}
	if err := resolution.Invocation.Validate(true); err != nil {
		return fmt.Errorf("runs: approval Tool invocation: %w", err)
	}
	if !resolution.Decision.Valid() {
		return fmt.Errorf("runs: invalid Tool approval decision %q", resolution.Decision)
	}
	return nil
}

func (claim ResumeClaimCommit) Validate() error {
	if strings.TrimSpace(claim.CommitID) == "" || claim.CommitID != strings.TrimSpace(claim.CommitID) {
		return errors.New("runs: resume claim commit identity is required without surrounding whitespace")
	}
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
	if _, err := claim.QuestionReplacements(); err != nil {
		return fmt.Errorf("runs: resume claim question projections: %w", err)
	}
	if _, err := claim.ToolApprovalResolutions(); err != nil {
		return fmt.Errorf("runs: resume claim Tool approval projections: %w", err)
	}
	return nil
}

// ToolApprovalResolutions derives the durable verdict for every accepted
// approval response from the exact Pending snapshot. It deliberately carries
// the original prompt invocation so the answer claim validates the exact
// reviewed boundary. Edited arguments become the approved execution input and
// may therefore replace the invocation on the terminal Tool Item; Item and
// provider call identities, rather than mutable arguments, preserve continuity.
func (claim ResumeClaimCommit) ToolApprovalResolutions() ([]ToolApprovalResolution, error) {
	answersByItem := make(map[string]InterruptAnswer, len(claim.Answers))
	bindingsByItem := make(map[string]InterruptBinding, len(claim.Expected.Bindings))
	for _, answer := range claim.Answers {
		answersByItem[answer.InterruptItemID] = answer
	}
	for _, binding := range claim.Expected.Bindings {
		bindingsByItem[binding.InterruptItemID] = binding
	}
	resolutions := make([]ToolApprovalResolution, 0, len(claim.Expected.Interrupts))
	for _, request := range claim.Expected.Interrupts {
		if request.Kind != interrupt.Approval {
			continue
		}
		if request.Approval == nil {
			return nil, fmt.Errorf("approval item %q has no prompt", request.ItemID)
		}
		answer, ok := answersByItem[request.ItemID]
		if !ok {
			return nil, fmt.Errorf("approval item %q has no answer", request.ItemID)
		}
		binding, ok := bindingsByItem[request.ItemID]
		if !ok {
			return nil, fmt.Errorf("approval item %q has no continuation binding", request.ItemID)
		}
		resolution := ToolApprovalResolution{
			Identity: transcript.ItemIdentity{
				SessionID: claim.Expected.SessionID, RunID: request.RunID,
				ItemID: request.ItemID, OccurredAt: request.ItemOccurredAt,
			},
			CallID:     binding.ToolCallID,
			Invocation: request.Approval.Tool,
			Decision:   approval.DecisionOf(answer.Resolution.Approved),
		}
		if err := resolution.Validate(); err != nil {
			return nil, fmt.Errorf("approval item %q: %w", request.ItemID, err)
		}
		resolutions = append(resolutions, resolution)
	}
	return resolutions, nil
}

// QuestionReplacements derives the transcript compare-and-swap write-set for
// every accepted Question answer. It is computed by the Application from the
// exact Pending snapshot and validated resolutions; the persistence port only
// executes these replacements in the same transaction as the claim.
func (claim ResumeClaimCommit) QuestionReplacements() ([]ItemReplacement, error) {
	answersByItem := make(map[string]InterruptAnswer, len(claim.Answers))
	for _, answer := range claim.Answers {
		answersByItem[answer.InterruptItemID] = answer
	}
	replacements := make([]ItemReplacement, 0, len(claim.Expected.Interrupts))
	for _, request := range claim.Expected.Interrupts {
		if request.Kind != interrupt.Question {
			continue
		}
		if request.Question == nil {
			return nil, fmt.Errorf("question item %q has no prompt", request.ItemID)
		}
		answer, ok := answersByItem[request.ItemID]
		if !ok {
			return nil, fmt.Errorf("question item %q has no answer", request.ItemID)
		}
		expected, err := transcript.NewQuestion(transcript.ItemIdentity{
			SessionID:  claim.Expected.SessionID,
			RunID:      request.RunID,
			ItemID:     request.ItemID,
			OccurredAt: request.ItemOccurredAt,
		}, *request.Question)
		if err != nil {
			return nil, fmt.Errorf("restore question item %q: %w", request.ItemID, err)
		}
		replacement, err := expected.AnswerQuestion(answer.Resolution.Answers)
		if err != nil {
			return nil, fmt.Errorf("answer question item %q: %w", request.ItemID, err)
		}
		replacements = append(replacements, ItemReplacement{
			Expected: expected, Replacement: replacement,
		})
	}
	return replacements, nil
}

// SessionReplacement is the exact Session aggregate replacement committed with
// the Run admission whose configured model produced it.
type SessionReplacement struct {
	ExpectedRevision uint64
	State            session.Session
}

// Validate proves that the replacement advances the same Session once.
func (r SessionReplacement) Validate(sessionID string) error {
	if err := r.State.Validate(); err != nil {
		return fmt.Errorf("runs: invalid Session replacement: %w", err)
	}
	if r.State.ID() != sessionID {
		return errors.New("runs: opening Session replacement differs from admitted Run")
	}
	if r.ExpectedRevision == 0 || r.ExpectedRevision == ^uint64(0) ||
		r.State.Revision() != r.ExpectedRevision+1 {
		return errors.New("runs: opening Session replacement does not advance one revision")
	}
	return nil
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
	Metrics   run.Metrics
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
	// SegmentID owns the complete event write-set, including projections that do
	// not otherwise carry segment identity. Persistence admits the transaction
	// only while this exact Segment is still active for the Run.
	SegmentID string
	// CommitID is the stable identity of one immutable top-level CommitEvent
	// write-set. Persistence records it inside that transaction, allowing a lost
	// COMMIT receipt to be reconciled without treating another Segment or write
	// attempt as success. Nested opening/barrier projections may leave it empty;
	// the top-level CommitEvent port boundary requires it.
	CommitID string
	State    StateChange
	Outcome  run.Outcome
	Items    []transcript.Item
	// ConversationMessages are the provider-neutral messages this root
	// execution made durable for future model context. Conversation and
	// Transcript remain separate projections: the former feeds later model
	// calls, while the latter owns user-visible Run history.
	ConversationMessages []corechat.Message
	// ModelInvocations, ToolInvocations, and Progress are application
	// observations committed in the same transaction as the semantic Transcript
	// Items derived from one authoritative executor fact.
	ModelInvocations []ModelInvocationCommit
	ToolInvocations  []ToolInvocationCommit
	Progress         *RunProgressCommit
	Run              *run.Run
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
	if err := c.validateConversationMessages(); err != nil {
		return err
	}
	if err := c.validateInvocations(); err != nil {
		return err
	}
	if c.Progress != nil {
		if err := c.Progress.validate(); err != nil {
			return err
		}
		if c.Progress.SegmentID != c.SegmentID {
			return fmt.Errorf("runs: event commit progress belongs to Segment %q, want %q", c.Progress.SegmentID, c.SegmentID)
		}
	}
	return c.validateLifecycle()
}

func (c EventCommit) validateConversationMessages() error {
	for index, message := range c.ConversationMessages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("runs: event commit conversation message[%d]: %w", index, err)
		}
	}
	return nil
}

func (c EventCommit) validateEnvelope() error {
	if strings.TrimSpace(c.RunID) == "" || c.RunID != strings.TrimSpace(c.RunID) {
		return errors.New("runs: event commit Run ID must be non-empty without surrounding whitespace")
	}
	if strings.TrimSpace(c.SessionID) == "" || c.SessionID != strings.TrimSpace(c.SessionID) {
		return errors.New("runs: event commit Session ID must be non-empty without surrounding whitespace")
	}
	if strings.TrimSpace(c.SegmentID) == "" || c.SegmentID != strings.TrimSpace(c.SegmentID) {
		return errors.New("runs: event commit Segment ID must be non-empty without surrounding whitespace")
	}
	if c.ObsoleteCheckpointRootID != strings.TrimSpace(c.ObsoleteCheckpointRootID) {
		return errors.New("runs: event commit checkpoint root ID has surrounding whitespace")
	}
	if c.CommitID != strings.TrimSpace(c.CommitID) {
		return errors.New("runs: event commit identity has surrounding whitespace")
	}
	return nil
}

func (c EventCommit) validateItems() error {
	seenItems := make(map[string]struct{}, len(c.Items))
	for index, item := range c.Items {
		if item.ID() == "" || item.RunID() != c.RunID || item.SessionID() != c.SessionID {
			return fmt.Errorf("runs: event commit Item[%d] is not owned by Run %q", index, c.RunID)
		}
		if _, duplicate := seenItems[item.ID()]; duplicate {
			return fmt.Errorf("runs: event commit repeats Item %q", item.ID())
		}
		seenItems[item.ID()] = struct{}{}
		if err := item.Validate(); err != nil {
			return fmt.Errorf("runs: event commit Item %q: %w", item.ID(), err)
		}
	}
	return nil
}

func (c EventCommit) validateInvocations() error {
	items := make(map[string]transcript.Item, len(c.Items))
	for _, item := range c.Items {
		items[item.ID()] = item
	}
	seenInvocations := make(map[string]struct{}, len(c.ModelInvocations))
	for index, invocation := range c.ModelInvocations {
		if err := invocation.validate(); err != nil {
			return fmt.Errorf("runs: event commit model invocation[%d]: %w", index, err)
		}
		if _, duplicate := seenInvocations[invocation.CallID]; duplicate {
			return fmt.Errorf("runs: event commit repeats model invocation %q", invocation.CallID)
		}
		if invocation.SegmentID != c.SegmentID {
			return fmt.Errorf("runs: event commit model invocation[%d] belongs to Segment %q, want %q", index, invocation.SegmentID, c.SegmentID)
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
		if invocation.SegmentID != c.SegmentID {
			return fmt.Errorf("runs: event commit Tool invocation[%d] belongs to Segment %q, want %q", index, invocation.SegmentID, c.SegmentID)
		}
		if _, duplicate := seenToolItems[invocation.ItemID]; duplicate {
			return fmt.Errorf("runs: event commit repeats Tool invocation Item %q", invocation.ItemID)
		}
		seenTools[invocation.CallID] = struct{}{}
		seenToolItems[invocation.ItemID] = struct{}{}
		item, present := items[invocation.ItemID]
		if !present || item.Kind() != transcript.ToolCall {
			return fmt.Errorf(
				"runs: event commit Tool invocation %q has no matching Tool Item",
				invocation.CallID,
			)
		}
		switch invocation.State {
		case ToolInvocationStarted:
			if item.Status() != transcript.ItemRunning {
				return fmt.Errorf("runs: started Tool invocation %q Item is not running", invocation.CallID)
			}
		case ToolInvocationCompleted:
			switch item.Status() {
			case transcript.ItemCompleted:
			case transcript.ItemIncomplete:
				if _, failed := item.Failure(); !failed {
					return fmt.Errorf("runs: completed Tool invocation %q has an unclassified incomplete Item", invocation.CallID)
				}
			default:
				return fmt.Errorf("runs: completed Tool invocation %q Item is not terminal", invocation.CallID)
			}
		case ToolInvocationIncomplete:
			if item.Status() != transcript.ItemIncomplete && item.Status() != transcript.ItemRunning {
				return fmt.Errorf("runs: incomplete Tool invocation %q Item is neither incomplete nor parked", invocation.CallID)
			}
		}
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
		if c.Run == nil || c.Run.State() != run.Waiting {
			return errors.New("runs: suspend event commit has no waiting Run")
		}
		if c.GoalRun != nil || c.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: suspend event commit carries terminal facts")
		}
	case StateTerminalize:
		if c.CommitID == "" {
			return errors.New("runs: terminal event commit has no commit identity")
		}
		if c.Run == nil || !c.Run.State().IsTerminal() {
			return errors.New("runs: terminal event commit has no matching terminal Run")
		}
		outcome, ok := c.Run.Outcome()
		if !ok || outcome != c.Outcome {
			return errors.New("runs: terminal event commit has no matching terminal outcome")
		}
	default:
		return fmt.Errorf("runs: event commit has unknown state change %d", c.State)
	}

	if c.Run.ID() != c.RunID || c.Run.SessionID() != c.SessionID {
		return errors.New("runs: event commit Run ownership differs from its envelope")
	}
	validatedRun := *c.Run
	if c.State == StateTerminalize && validatedRun.MessageMark() == run.UnknownMessageMark {
		// The reducer cannot know the final conversation watermark. The terminal
		// transaction resolves it while committing this Run; every other terminal
		// fact must already satisfy the domain invariant.
		var err error
		validatedRun, err = validatedRun.WithMessageMark(0)
		if err != nil {
			return fmt.Errorf("runs: resolve provisional message watermark: %w", err)
		}
	}
	if err := validatedRun.Validate(); err != nil {
		return fmt.Errorf("runs: event commit Run: %w", err)
	}
	if c.State == StateSuspend {
		return nil
	}
	return validateTerminalGoalRun(*c.Run, c.GoalRun)
}

func validateTerminalGoalRun(value run.Run, record *goal.RunRecord) error {
	if value.GoalIncarnationID() == "" {
		if record != nil {
			return fmt.Errorf("runs: non-Goal Run %q carries a Goal Run", value.ID())
		}
		return nil
	}
	if !value.Lineage().IsRoot() {
		return fmt.Errorf("runs: child Run %q carries a root Goal incarnation", value.ID())
	}
	if record == nil {
		return fmt.Errorf("runs: Goal-owned terminal Run %q has no Goal Run", value.ID())
	}
	if err := record.Validate(); err != nil {
		return fmt.Errorf("runs: terminal Goal Run: %w", err)
	}
	costUSD := 0.0
	if usage, ok := value.Metrics().Usage(); ok && usage.Total.CostUSD != nil {
		costUSD = *usage.Total.CostUSD
	}
	outcome, ok := value.Outcome()
	if !ok || record.SessionID != value.SessionID() || record.IncarnationID != value.GoalIncarnationID() ||
		record.RunID != value.ID() || record.Outcome != outcome || record.CostUSD != costUSD ||
		record.Steps != value.Metrics().Steps() || !record.CompletedAt.Equal(value.FinishedAt()) {
		return fmt.Errorf("runs: Goal Run differs from terminal Run %q", value.ID())
	}
	return nil
}

func (c EventCommit) isEmpty() bool {
	return len(c.Items) == 0 &&
		len(c.ConversationMessages) == 0 &&
		len(c.ModelInvocations) == 0 &&
		len(c.ToolInvocations) == 0 &&
		c.Progress == nil &&
		c.Run == nil &&
		c.GoalRun == nil &&
		c.ObsoleteCheckpointRootID == "" &&
		c.State == StateUnchanged
}

// Validate proves that the opening is exactly one fresh admission or one tree
// continuation and that every accompanying projection is limited to transcript
// Items and/or provider conversation messages. Those projections are deliberately
// independent: an application-authored Goal instruction may feed future model
// context without becoming a user-visible Item. Persistence Port implementations
// may reject unavailable stores or concurrent state changes, but they do not
// reinterpret this application write-set.
func (c OpeningCommit) Validate() error {
	if strings.TrimSpace(c.CommitID) == "" || c.CommitID != strings.TrimSpace(c.CommitID) {
		return errors.New("runs: opening commit identity is required without surrounding whitespace")
	}
	if (c.Admit == nil) == (c.Resume == nil) {
		return errors.New("runs: opening requires exactly one admission action")
	}
	if c.Admit != nil {
		if c.InitialSession != nil {
			if err := c.InitialSession.Validate(); err != nil {
				return fmt.Errorf("runs: opening initial Session: %w", err)
			}
			if c.InitialSession.ID() != c.Admit.SessionID || c.InitialSession.Revision() != 1 {
				return errors.New("runs: opening initial Session differs from admitted Run")
			}
		}
		if c.SessionReplacement != nil {
			if err := c.SessionReplacement.Validate(c.Admit.SessionID); err != nil {
				return err
			}
		}
		if c.InitialSession != nil && c.SessionReplacement != nil {
			return errors.New("runs: opening cannot insert and replace the same Session")
		}
		if c.ScheduleFiring != strings.TrimSpace(c.ScheduleFiring) {
			return errors.New("runs: opening schedule firing has surrounding whitespace")
		}
	} else if c.InitialSession != nil || c.SessionReplacement != nil || c.ScheduleFiring != "" {
		return errors.New("runs: resumed opening carries fresh-run facts")
	}
	for index, commit := range c.Events {
		if err := commit.Validate(); err != nil {
			return fmt.Errorf("runs: opening event[%d]: %w", index, err)
		}
		if commit.State != StateUnchanged ||
			(len(commit.Items) == 0 && len(commit.ConversationMessages) == 0) {
			return fmt.Errorf("runs: opening event[%d] has no transcript or conversation projection", index)
		}
	}
	return nil
}

// TreeBarrierCommit is the one durable write-set produced when any executor
// interruption stops a Run tree. Pending owns the complete continuation hand-off;
// Runs contains one StateSuspend commit for every active Run in deterministic
// postorder. No individual Run commit may write or consume the root-owned set.
type TreeBarrierCommit struct {
	CommitID   string
	Pending    Pending
	Runs       []EventCommit
	Checkpoint ExecutorCheckpoint
}

// Validate proves that the barrier is the complete interruption projection for
// the pending continuation tree and that its checkpoint belongs to the same
// run. The Effects port only persists this already-defined write-set.
func (c TreeBarrierCommit) Validate() error {
	if strings.TrimSpace(c.CommitID) == "" || c.CommitID != strings.TrimSpace(c.CommitID) {
		return errors.New("runs: tree barrier commit identity is required without surrounding whitespace")
	}
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
	if checkpoint.Scope.GoalIncarnationID != pending.GoalIncarnationID {
		return fmt.Errorf(
			"runs: tree barrier checkpoint goal incarnation %q does not match Pending %q: %w",
			checkpoint.Scope.GoalIncarnationID,
			pending.GoalIncarnationID,
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
	if runCommit.State != StateSuspend || runCommit.Run == nil || runCommit.Run.State() != run.Waiting {
		return fmt.Errorf("runs: tree barrier Run[%d] is not a waiting Run projection", index)
	}
	pending := validator.barrier.Pending
	if runCommit.SessionID != pending.SessionID || runCommit.Run.SessionID() != pending.SessionID {
		return fmt.Errorf("runs: tree barrier Run[%d] Session differs from Pending", index)
	}
	continuation, exists := validator.continuations[runCommit.RunID]
	if !exists {
		return fmt.Errorf("runs: tree barrier Run[%d] has no continuation", index)
	}
	if runCommit.Run.Lineage() != continuation.Lineage ||
		runCommit.Run.ModelSelection() != continuation.ModelSelection ||
		!runCommit.Run.CreatedAt().Equal(continuation.RunCreatedAt) ||
		!runCommit.Run.Metrics().Equal(continuation.Metrics) ||
		runCommit.Run.Limits() != continuation.Limits {
		return fmt.Errorf("runs: tree barrier Run[%d] differs from its continuation", index)
	}
	if !runCommit.Run.Capabilities().Equal(pending.Capabilities) {
		return fmt.Errorf("runs: tree barrier Run[%d] capabilities differ from Pending", index)
	}
	if runCommit.RunID == pending.RootRunID {
		if runCommit.Run.GoalIncarnationID() != pending.GoalIncarnationID {
			return errors.New("runs: tree barrier root Run goal incarnation differs from Pending")
		}
	} else if runCommit.Run.GoalIncarnationID() != "" {
		return fmt.Errorf("runs: tree barrier child Run[%d] carries a root Goal incarnation", index)
	}
	if _, duplicate := validator.seenRunIDs[runCommit.RunID]; duplicate {
		return fmt.Errorf("runs: tree barrier repeats Run %q", runCommit.RunID)
	}
	validator.seenRunIDs[runCommit.RunID] = struct{}{}
	return nil
}
