package runs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/approval"
	"github.com/Tangerg/scope/app/runtime/internal/domain/goal"
	"github.com/Tangerg/scope/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/session"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
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

func (t ToolApprovalResolution) Validate() error {
	if err := t.Identity.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(t.CallID) == "" || t.CallID != strings.TrimSpace(t.CallID) {
		return errors.New("runs: approval Tool call identity is required without surrounding whitespace")
	}
	if err := t.Invocation.Validate(true); err != nil {
		return fmt.Errorf("runs: approval Tool invocation: %w", err)
	}
	if !t.Decision.Valid() {
		return fmt.Errorf("runs: invalid Tool approval decision %q", t.Decision)
	}
	return nil
}

func (r ResumeClaimCommit) Validate() error {
	if strings.TrimSpace(r.CommitID) == "" || r.CommitID != strings.TrimSpace(r.CommitID) {
		return errors.New("runs: resume claim commit identity is required without surrounding whitespace")
	}
	if err := r.Expected.Validate(); err != nil {
		return fmt.Errorf("runs: resume claim Pending: %w", err)
	}
	if r.ClaimedAt.IsZero() {
		return errors.New("runs: resume claim time is required")
	}
	if len(r.Answers) != len(r.Expected.Bindings) {
		return fmt.Errorf(
			"runs: resume claim has %d answers for %d boundaries",
			len(r.Answers), len(r.Expected.Bindings),
		)
	}
	for index, answer := range r.Answers {
		binding := r.Expected.Bindings[index]
		if answer.InterruptItemID != binding.InterruptItemID || answer.MemberID != binding.MemberID ||
			answer.RequestID != binding.RequestID {
			return fmt.Errorf("runs: resume claim answer[%d] differs from its pending boundary", index)
		}
	}
	if _, err := r.QuestionReplacements(); err != nil {
		return fmt.Errorf("runs: resume claim question projections: %w", err)
	}
	if _, err := r.ToolApprovalResolutions(); err != nil {
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
func (r ResumeClaimCommit) ToolApprovalResolutions() ([]ToolApprovalResolution, error) {
	answersByItem := make(map[string]InterruptAnswer, len(r.Answers))
	bindingsByItem := make(map[string]InterruptBinding, len(r.Expected.Bindings))
	for _, answer := range r.Answers {
		answersByItem[answer.InterruptItemID] = answer
	}
	for _, binding := range r.Expected.Bindings {
		bindingsByItem[binding.InterruptItemID] = binding
	}
	resolutions := make([]ToolApprovalResolution, 0, len(r.Expected.Interrupts))
	for _, request := range r.Expected.Interrupts {
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
				SessionID: r.Expected.SessionID, RunID: request.RunID,
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
func (r ResumeClaimCommit) QuestionReplacements() ([]ItemReplacement, error) {
	answersByItem := make(map[string]InterruptAnswer, len(r.Answers))
	for _, answer := range r.Answers {
		answersByItem[answer.InterruptItemID] = answer
	}
	replacements := make([]ItemReplacement, 0, len(r.Expected.Interrupts))
	for _, request := range r.Expected.Interrupts {
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
			SessionID:  r.Expected.SessionID,
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
func (s SessionReplacement) Validate(sessionID string) error {
	if err := s.State.Validate(); err != nil {
		return fmt.Errorf("runs: invalid Session replacement: %w", err)
	}
	if s.State.ID() != sessionID {
		return errors.New("runs: opening Session replacement differs from admitted Run")
	}
	if s.ExpectedRevision == 0 || s.ExpectedRevision == ^uint64(0) ||
		s.State.Revision() != s.ExpectedRevision+1 {
		return errors.New("runs: opening Session replacement does not advance one revision")
	}
	return nil
}

type StateChange string

const (
	// StateUnchanged is the meaningful zero value: the commit advances other
	// durable Run facts without moving the lifecycle state.
	StateUnchanged   StateChange = ""
	StateSuspend     StateChange = "suspend"
	StateTerminalize StateChange = "terminalize"
)

// Valid reports whether s is one supported lifecycle mutation.
func (s StateChange) Valid() bool {
	return s == StateUnchanged || s == StateSuspend || s == StateTerminalize
}

// ModelInvocationState records the durable application observation of one
// provider call. It is deliberately smaller than a model response: semantic
// output belongs to Transcript Items and accounting belongs to RunProgressCommit.
// This record exists to distinguish an invocation that never crossed the
// provider boundary from one whose final projection became indeterminate.
type ModelInvocationState string

const (
	ModelInvocationStarted   ModelInvocationState = "started"
	ModelInvocationCompleted ModelInvocationState = "completed"
	ModelInvocationFailed    ModelInvocationState = "failed"
	ModelInvocationUnknown   ModelInvocationState = "unknown"
)

// Valid reports whether m belongs to the durable model-invocation journal.
func (m ModelInvocationState) Valid() bool {
	return m == ModelInvocationStarted || m == ModelInvocationCompleted ||
		m == ModelInvocationFailed || m == ModelInvocationUnknown
}

// String returns the durable model-invocation state name.
func (m ModelInvocationState) String() string {
	if !m.Valid() {
		return "invalid"
	}
	return string(m)
}

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
type ToolInvocationState string

const (
	ToolInvocationStarted    ToolInvocationState = "started"
	ToolInvocationCompleted  ToolInvocationState = "completed"
	ToolInvocationIncomplete ToolInvocationState = "incomplete"
)

// Valid reports whether t belongs to the durable Tool-invocation journal.
func (t ToolInvocationState) Valid() bool {
	return t == ToolInvocationStarted || t == ToolInvocationCompleted || t == ToolInvocationIncomplete
}

// String returns the durable Tool-invocation state name.
func (t ToolInvocationState) String() string {
	if !t.Valid() {
		return "invalid"
	}
	return string(t)
}

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

func (t ToolInvocationCommit) validate() error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: "call", value: t.CallID},
		{name: "Item", value: t.ItemID},
		{name: "segment", value: t.SegmentID},
	} {
		name, value := identity.name, identity.value
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("runs: Tool invocation %s ID is required without surrounding whitespace", name)
		}
	}
	if t.StartedAt.IsZero() {
		return errors.New("runs: Tool invocation start time is required")
	}
	switch t.State {
	case ToolInvocationStarted:
		if !t.FinishedAt.IsZero() {
			return errors.New("runs: started Tool invocation carries a finish time")
		}
	case ToolInvocationCompleted, ToolInvocationIncomplete:
		if t.FinishedAt.IsZero() {
			return errors.New("runs: terminal Tool invocation has no finish time")
		}
		if t.FinishedAt.Before(t.StartedAt) {
			return errors.New("runs: Tool invocation finish time precedes start time")
		}
	default:
		return fmt.Errorf("runs: Tool invocation has unknown state %q", t.State)
	}
	return nil
}

func (m ModelInvocationCommit) validate() error {
	if strings.TrimSpace(m.CallID) == "" || m.CallID != strings.TrimSpace(m.CallID) {
		return errors.New("runs: model invocation call ID is required without surrounding whitespace")
	}
	if strings.TrimSpace(m.SegmentID) == "" || m.SegmentID != strings.TrimSpace(m.SegmentID) {
		return errors.New("runs: model invocation segment ID is required without surrounding whitespace")
	}
	if m.StartedAt.IsZero() {
		return errors.New("runs: model invocation start time is required")
	}
	switch m.State {
	case ModelInvocationStarted:
		if !m.FinishedAt.IsZero() {
			return errors.New("runs: started model invocation carries a finish time")
		}
	case ModelInvocationCompleted, ModelInvocationFailed, ModelInvocationUnknown:
		if m.FinishedAt.IsZero() {
			return errors.New("runs: terminal model invocation has no finish time")
		}
		if m.FinishedAt.Before(m.StartedAt) {
			return errors.New("runs: model invocation finish time precedes start time")
		}
	default:
		return fmt.Errorf("runs: model invocation has unknown state %q", m.State)
	}
	return nil
}

// RunProgressCommit is the durable progress snapshot produced at a model-response
// boundary. Metrics are cumulative; ContextTokens is the latest prompt footprint
// and may decrease after compaction. SegmentID fences both facts to the exact
// running segment so a stale continuation cannot overwrite a newer Run.
type RunProgressCommit struct {
	SegmentID     string
	Metrics       run.Metrics
	ContextTokens int64
	UpdatedAt     time.Time
}

func (r RunProgressCommit) validate() error {
	if strings.TrimSpace(r.SegmentID) == "" || r.SegmentID != strings.TrimSpace(r.SegmentID) {
		return errors.New("runs: progress segment ID is required without surrounding whitespace")
	}
	if r.UpdatedAt.IsZero() {
		return errors.New("runs: progress update time is required")
	}
	if err := r.Metrics.Validate(); err != nil {
		return fmt.Errorf("runs: progress metrics: %w", err)
	}
	if r.ContextTokens < 0 {
		return errors.New("runs: progress context tokens must not be negative")
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
func (e EventCommit) Validate() error {
	if err := e.validateEnvelope(); err != nil {
		return err
	}
	if err := e.validateItems(); err != nil {
		return err
	}
	if err := e.validateConversationMessages(); err != nil {
		return err
	}
	if err := e.validateInvocations(); err != nil {
		return err
	}
	if e.Progress != nil {
		if err := e.Progress.validate(); err != nil {
			return err
		}
		if e.Progress.SegmentID != e.SegmentID {
			return fmt.Errorf("runs: event commit progress belongs to Segment %q, want %q", e.Progress.SegmentID, e.SegmentID)
		}
	}
	return e.validateLifecycle()
}

func (e EventCommit) validateConversationMessages() error {
	for index, message := range e.ConversationMessages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("runs: event commit conversation message[%d]: %w", index, err)
		}
	}
	return nil
}

func (e EventCommit) validateEnvelope() error {
	if strings.TrimSpace(e.RunID) == "" || e.RunID != strings.TrimSpace(e.RunID) {
		return errors.New("runs: event commit Run ID must be non-empty without surrounding whitespace")
	}
	if strings.TrimSpace(e.SessionID) == "" || e.SessionID != strings.TrimSpace(e.SessionID) {
		return errors.New("runs: event commit Session ID must be non-empty without surrounding whitespace")
	}
	if strings.TrimSpace(e.SegmentID) == "" || e.SegmentID != strings.TrimSpace(e.SegmentID) {
		return errors.New("runs: event commit Segment ID must be non-empty without surrounding whitespace")
	}
	if e.ObsoleteCheckpointRootID != strings.TrimSpace(e.ObsoleteCheckpointRootID) {
		return errors.New("runs: event commit checkpoint root ID has surrounding whitespace")
	}
	if e.CommitID != strings.TrimSpace(e.CommitID) {
		return errors.New("runs: event commit identity has surrounding whitespace")
	}
	return nil
}

func (e EventCommit) validateItems() error {
	seenItems := make(map[string]struct{}, len(e.Items))
	for index, item := range e.Items {
		if item.ID() == "" || item.RunID() != e.RunID || item.SessionID() != e.SessionID {
			return fmt.Errorf("runs: event commit Item[%d] is not owned by Run %q", index, e.RunID)
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

func (e EventCommit) validateInvocations() error {
	items := make(map[string]transcript.Item, len(e.Items))
	for _, item := range e.Items {
		items[item.ID()] = item
	}
	seenInvocations := make(map[string]struct{}, len(e.ModelInvocations))
	for index, invocation := range e.ModelInvocations {
		if err := invocation.validate(); err != nil {
			return fmt.Errorf("runs: event commit model invocation[%d]: %w", index, err)
		}
		if _, duplicate := seenInvocations[invocation.CallID]; duplicate {
			return fmt.Errorf("runs: event commit repeats model invocation %q", invocation.CallID)
		}
		if invocation.SegmentID != e.SegmentID {
			return fmt.Errorf("runs: event commit model invocation[%d] belongs to Segment %q, want %q", index, invocation.SegmentID, e.SegmentID)
		}
		seenInvocations[invocation.CallID] = struct{}{}
	}
	seenTools := make(map[string]struct{}, len(e.ToolInvocations))
	seenToolItems := make(map[string]struct{}, len(e.ToolInvocations))
	for index, invocation := range e.ToolInvocations {
		if err := invocation.validate(); err != nil {
			return fmt.Errorf("runs: event commit Tool invocation[%d]: %w", index, err)
		}
		if _, duplicate := seenTools[invocation.CallID]; duplicate {
			return fmt.Errorf("runs: event commit repeats Tool invocation %q", invocation.CallID)
		}
		if invocation.SegmentID != e.SegmentID {
			return fmt.Errorf("runs: event commit Tool invocation[%d] belongs to Segment %q, want %q", index, invocation.SegmentID, e.SegmentID)
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

func (e EventCommit) validateLifecycle() error {
	switch e.State {
	case StateUnchanged:
		if e.Run != nil || e.GoalRun != nil || e.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: unchanged event commit carries lifecycle facts")
		}
		return nil
	case StateSuspend:
		if e.Run == nil || e.Run.State() != run.Waiting {
			return errors.New("runs: suspend event commit has no waiting Run")
		}
		if e.GoalRun != nil || e.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: suspend event commit carries terminal facts")
		}
	case StateTerminalize:
		if e.CommitID == "" {
			return errors.New("runs: terminal event commit has no commit identity")
		}
		if e.Run == nil || !e.Run.State().IsTerminal() {
			return errors.New("runs: terminal event commit has no matching terminal Run")
		}
		outcome, ok := e.Run.Outcome()
		if !ok || outcome != e.Outcome {
			return errors.New("runs: terminal event commit has no matching terminal outcome")
		}
	default:
		return fmt.Errorf("runs: event commit has unknown state change %q", e.State)
	}

	if e.Run.ID() != e.RunID || e.Run.SessionID() != e.SessionID {
		return errors.New("runs: event commit Run ownership differs from its envelope")
	}
	validatedRun := *e.Run
	if e.State == StateTerminalize && validatedRun.MessageMark() == run.UnknownMessageMark {
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
	if e.State == StateSuspend {
		return nil
	}
	return validateTerminalGoalRun(*e.Run, e.GoalRun)
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

func (e EventCommit) isEmpty() bool {
	return len(e.Items) == 0 &&
		len(e.ConversationMessages) == 0 &&
		len(e.ModelInvocations) == 0 &&
		len(e.ToolInvocations) == 0 &&
		e.Progress == nil &&
		e.Run == nil &&
		e.GoalRun == nil &&
		e.ObsoleteCheckpointRootID == "" &&
		e.State == StateUnchanged
}

// Validate proves that the opening is exactly one fresh admission or one tree
// continuation and that every accompanying projection is limited to transcript
// Items and/or provider conversation messages. Those projections are deliberately
// independent: an application-authored Goal instruction may feed future model
// context without becoming a user-visible Item. Persistence Port implementations
// may reject unavailable stores or concurrent state changes, but they do not
// reinterpret this application write-set.
func (o OpeningCommit) Validate() error {
	if strings.TrimSpace(o.CommitID) == "" || o.CommitID != strings.TrimSpace(o.CommitID) {
		return errors.New("runs: opening commit identity is required without surrounding whitespace")
	}
	if (o.Admit == nil) == (o.Resume == nil) {
		return errors.New("runs: opening requires exactly one admission action")
	}
	if o.Admit != nil {
		if o.InitialSession != nil {
			if err := o.InitialSession.Validate(); err != nil {
				return fmt.Errorf("runs: opening initial Session: %w", err)
			}
			if o.InitialSession.ID() != o.Admit.SessionID || o.InitialSession.Revision() != 1 {
				return errors.New("runs: opening initial Session differs from admitted Run")
			}
		}
		if o.SessionReplacement != nil {
			if err := o.SessionReplacement.Validate(o.Admit.SessionID); err != nil {
				return err
			}
		}
		if o.InitialSession != nil && o.SessionReplacement != nil {
			return errors.New("runs: opening cannot insert and replace the same Session")
		}
		if o.ScheduleFiring != strings.TrimSpace(o.ScheduleFiring) {
			return errors.New("runs: opening schedule firing has surrounding whitespace")
		}
	} else if o.InitialSession != nil || o.SessionReplacement != nil || o.ScheduleFiring != "" {
		return errors.New("runs: resumed opening carries fresh-run facts")
	}
	for index, commit := range o.Events {
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
func (t TreeBarrierCommit) Validate() error {
	if strings.TrimSpace(t.CommitID) == "" || t.CommitID != strings.TrimSpace(t.CommitID) {
		return errors.New("runs: tree barrier commit identity is required without surrounding whitespace")
	}
	if err := t.Pending.Validate(); err != nil {
		return fmt.Errorf("runs: tree barrier Pending: %w", err)
	}
	rootContinuation, found := t.Pending.RootContinuation()
	if !found {
		return errors.New("runs: tree barrier has no root continuation")
	}
	validator := treeBarrierValidator{
		barrier:       t,
		continuations: make(map[string]Continuation, len(t.Pending.Continuations)),
		seenRunIDs:    make(map[string]struct{}, len(t.Runs)),
	}
	for _, continuation := range t.Pending.Continuations {
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

func (t treeBarrierValidator) validateCheckpoint(rootContinuation Continuation) error {
	checkpoint := t.barrier.Checkpoint
	pending := t.barrier.Pending
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

func (t treeBarrierValidator) validateRuns() error {
	if len(t.barrier.Runs) != len(t.barrier.Pending.Continuations) {
		return fmt.Errorf(
			"runs: tree barrier has %d Run commits for %d continuations",
			len(t.barrier.Runs),
			len(t.barrier.Pending.Continuations),
		)
	}
	for index, runCommit := range t.barrier.Runs {
		if err := t.validateRun(index, runCommit); err != nil {
			return err
		}
	}
	return nil
}

func (t treeBarrierValidator) validateRun(index int, runCommit EventCommit) error {
	if err := runCommit.Validate(); err != nil {
		return fmt.Errorf("runs: tree barrier Run[%d]: %w", index, err)
	}
	if runCommit.State != StateSuspend || runCommit.Run == nil || runCommit.Run.State() != run.Waiting {
		return fmt.Errorf("runs: tree barrier Run[%d] is not a waiting Run projection", index)
	}
	pending := t.barrier.Pending
	if runCommit.SessionID != pending.SessionID || runCommit.Run.SessionID() != pending.SessionID {
		return fmt.Errorf("runs: tree barrier Run[%d] Session differs from Pending", index)
	}
	continuation, exists := t.continuations[runCommit.RunID]
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
	if _, duplicate := t.seenRunIDs[runCommit.RunID]; duplicate {
		return fmt.Errorf("runs: tree barrier repeats Run %q", runCommit.RunID)
	}
	t.seenRunIDs[runCommit.RunID] = struct{}{}
	return nil
}
