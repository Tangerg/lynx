// Package runsegment is the driven adapter that executes the durable side
// effects of one streamed run segment. It implements the application's
// runs.Effects port: the run pump hands it a [runs.EventCommit] per event,
// which it applies ATOMICALLY — the open-interrupt record, transcript
// projections, and the run-state transition land in one transaction (§8.3/§8.4),
// so a crash never leaves a parked run with no admission mark or a terminal
// transcript with a still-running row. It also runs the non-durable live
// workspace nudge and terminal boundary maintenance (checkpoint snapshot,
// title). The fields only the runtime can resolve — an interrupt's executor member ID
// from live execution, a terminal Run's message watermark — it fills in itself.
package runsegment

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/goal"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/session"
	"github.com/Tangerg/scope/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/scope/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/scope/core/chat"
)

// SessionStore is the exact persistence surface used inside an opening
// transaction. Application and Domain have already decided each aggregate.
type SessionStore interface {
	Insert(ctx context.Context, value session.Session) error
	Save(ctx context.Context, expectedRevision uint64, replacement session.Session) error
}

// ScheduleFiringStore confirms the durable occurrence that owns a scheduled
// run. The confirmation shares the opening transaction with run admission, so
// an accepted occurrence and its Run cannot diverge across a crash.
type ScheduleFiringStore interface {
	Accept(ctx context.Context, occurrenceID, runID string) error
}

// GoalRunRecorder records the budget charge for a terminal goal-owned Run. It
// runs in the same transaction as terminalizing that Run.
type GoalRunRecorder interface {
	RecordRun(ctx context.Context, record goal.RunRecord) error
}

// InterruptStore is the run-segment lifecycle view of the interrupt registry.
// It opens a new quiescent barrier and removes any prior answer claim when the
// owning root terminalizes. Resume claiming remains a narrower, separate port.
type InterruptStore interface {
	Open(ctx context.Context, p runs.Pending) error
	Consume(ctx context.Context, sessionID, rootRunID string) (runs.Pending, bool, error)
	Delete(ctx context.Context, sessionID, rootRunID string) error
}

// ResumeClaimStore atomically changes one open hand-off into a durable,
// nonrecoverable answer claim. It is separate from ordinary barrier mutation so
// callers that never resume a Run do not acquire that lifecycle capability.
type ResumeClaimStore interface {
	ClaimResume(
		ctx context.Context,
		sessionID, rootRunID string,
		answers []runs.InterruptAnswer,
		claimedAt time.Time,
	) (runs.Pending, bool, error)
	RequireResumeClaim(ctx context.Context, sessionID, rootRunID string) error
}

// TranscriptStore is the run-segment append side of durable transcript
// persistence. Reading and destructive deletion belong to other use-cases.
type TranscriptStore interface {
	AppendItem(ctx context.Context, it transcript.Item) error
}

type ItemReplacer interface {
	ReplaceItem(ctx context.Context, expected transcript.Item, replacement transcript.Item) error
}

// ToolApprovalStore is the exact transcript read/CAS surface used at the
// answer-claim transaction. The adapter loads the running ToolCall, applies its
// domain transition, and replaces that same immutable value atomically.
type ToolApprovalStore interface {
	Item(ctx context.Context, itemID string) (transcript.Item, bool, error)
	ReplaceItem(ctx context.Context, expected transcript.Item, replacement transcript.Item) error
}

type ToolResultStore interface {
	Bind(ctx context.Context, sessionID, itemID, preview string, ref toolresult.Ref) error
	Discard(ctx context.Context, sessionID string, ref toolresult.Ref) error
}

// ModelInvocationJournal applies provider-attempt transitions. It is an
// operational journal, not a second copy of semantic Transcript output or Run
// accounting. The consumer-owned methods keep application enums out of storage.
type ModelInvocationJournal interface {
	StartModelInvocation(
		ctx context.Context,
		sessionID, runID, segmentID, callID string,
		startedAt time.Time,
	) error
	CompleteModelInvocation(
		ctx context.Context,
		sessionID, runID, segmentID, callID string,
		startedAt, finishedAt time.Time,
	) error
	FailModelInvocation(
		ctx context.Context,
		sessionID, runID, segmentID, callID string,
		startedAt, finishedAt time.Time,
	) error
	MarkModelInvocationUnknown(
		ctx context.Context,
		sessionID, runID, segmentID, callID string,
		startedAt, finishedAt time.Time,
	) error
}

// ToolInvocationJournal records pre-call and terminal attempt boundaries without
// using Transcript insertion order as an operational lock. Final semantic Tool
// Items are still committed through TranscriptStore in canonical model order.
type ToolInvocationJournal interface {
	StartToolInvocation(
		ctx context.Context,
		sessionID, runID, segmentID, callID, itemID string,
		startedAt time.Time,
	) error
	CompleteToolInvocation(
		ctx context.Context,
		sessionID, runID, segmentID, callID, itemID string,
		startedAt, finishedAt time.Time,
	) error
	MarkToolInvocationIncomplete(
		ctx context.Context,
		sessionID, runID, segmentID, callID, itemID string,
		startedAt, finishedAt time.Time,
	) error
}

// RunWriter applies the run's lifecycle transitions inside the event commit
// (§8.3): an opening admits or resumes it, a park suspends it, and a terminal
// ends it — each in the SAME transaction as the interrupt / item records it must
// stay consistent with. Suspend and Terminalize both take the whole Run because
// a state change and the accounting true at that moment are one fact: a park is
// a durable commit, so what the Run had spent by then is committed with it. The
// RequireActiveSegment is the admission fence for every EventCommit: it proves
// the complete write-set still belongs to the exact running Segment before any
// transcript, conversation, invocation, accounting, or lifecycle fact moves.
// RecordRunCommit records the Application-owned immutable write-set identity
// after a non-terminal command's complete projections in the same transaction.
// SuspendBarrier and TerminalizeEvent record it in their boundary transitions.
// The sqlite RunStore satisfies it.
type RunWriter interface {
	Admit(ctx context.Context, draft run.Draft) error
	Resume(ctx context.Context, sessionID string, draft run.ResumeDraft, resumedAt time.Time) error
	RequireActiveSegment(ctx context.Context, sessionID, runID, segmentID string) error
	Suspend(ctx context.Context, run run.Run) error
	SuspendBarrier(ctx context.Context, run run.Run, segmentID, commitID string) error
	Terminalize(ctx context.Context, run run.Run) error
	RecordRunCommit(ctx context.Context, sessionID, runID, segmentID, commitID string) error
	RecordWaitingRunCommit(ctx context.Context, sessionID, runID, commitID string) error
	TerminalizeEvent(ctx context.Context, run run.Run, segmentID, commitID string) error
}

// RunStore combines lifecycle writes with the exact durable reads required to
// prove and reconstruct a result after an ambiguous command commit.
type RunStore interface {
	RunWriter
	Run(ctx context.Context, runID string) (run.Run, bool, error)
	RunCommitCommitted(ctx context.Context, sessionID, runID, segmentID, commitID string) (bool, error)
}

// RunProgressWriter updates cumulative consumption and latest prompt footprint
// for one exact active segment. Keeping it separate from lifecycle writing lets
// consumers depend on the narrower behavior they actually exercise.
type RunProgressWriter interface {
	UpdateProgress(
		ctx context.Context,
		sessionID string,
		runID string,
		segmentID string,
		metrics run.Metrics,
		contextTokens int64,
		updatedAt time.Time,
	) error
}

// ExecutorCheckpointStore persists and removes root-owned opaque executor
// checkpoints selected by the Run lifecycle. It never interprets the
// payload.
type ExecutorCheckpointStore interface {
	SaveCheckpoint(ctx context.Context, checkpoint runs.ExecutorCheckpoint) error
	LoadCheckpoint(ctx context.Context, rootMemberID string) (runs.ExecutorCheckpoint, error)
	DeleteCheckpoints(ctx context.Context, sessionID string, rootIDs []string) error
}

// ChildRunStartReservationStore is the technical persistence mechanism behind
// the Application-owned child-start write-set. Its payload is opaque to SQLite;
// this adapter alone translates the application value.
type ChildRunStartReservationStore interface {
	Reserve(ctx context.Context, record sqlite.ChildRunStartReservationRecord) error
	Conclude(
		ctx context.Context,
		record sqlite.ChildRunStartReservationRecord,
		conclusion sqlite.ChildRunStartConclusion,
	) (changed bool, err error)
	DeleteSession(ctx context.Context, sessionID string) error
}

// Transactor runs fn inside one storage transaction: every store call made by
// fn joins that transaction through the context. Durable commits reject a nil
// transactor rather than silently weakening atomicity.
type Transactor func(ctx context.Context, fn func(context.Context) error) error

// ConversationStore appends root execution context and resolves the watermark
// recorded by a terminal Run. Both operations join the event transaction.
type ConversationStore interface {
	Write(ctx context.Context, sessionID string, messages ...chat.Message) error
	Count(ctx context.Context, sessionID string) (int, error)
}

// Config bundles the Effects dependencies.
type Config struct {
	Interrupts          InterruptStore
	ResumeClaims        ResumeClaimStore
	Sessions            SessionStore
	ScheduleFirings     ScheduleFiringStore
	GoalRuns            GoalRunRecorder
	Transcript          TranscriptStore
	ItemReplacer        ItemReplacer
	ToolApprovals       ToolApprovalStore
	ToolResults         ToolResultStore
	ModelInvocations    ModelInvocationJournal
	ToolInvocations     ToolInvocationJournal
	Conversation        ConversationStore
	State               RunStore
	RunProgress         RunProgressWriter
	ExecutorCheckpoints ExecutorCheckpointStore
	ChildRunStarts      ChildRunStartReservationStore
	Tx                  Transactor
}

// Effects coordinates run-segment side effects. It is stateless beyond its
// dependencies and safe to share.
type Effects struct {
	interrupts          InterruptStore
	resumeClaims        ResumeClaimStore
	sessions            SessionStore
	scheduleFirings     ScheduleFiringStore
	goalRuns            GoalRunRecorder
	transcript          TranscriptStore
	itemReplacer        ItemReplacer
	toolApprovals       ToolApprovalStore
	toolResults         ToolResultStore
	modelInvocations    ModelInvocationJournal
	toolInvocations     ToolInvocationJournal
	conversation        ConversationStore
	runState            RunStore
	runProgress         RunProgressWriter
	executorCheckpoints ExecutorCheckpointStore
	childRunStarts      ChildRunStartReservationStore
	tx                  Transactor
}

var (
	_ runs.OpeningCommitter                    = (*Effects)(nil)
	_ runs.ChildRunStartCommitter              = (*Effects)(nil)
	_ runs.ResumeClaimCommitter                = (*Effects)(nil)
	_ runs.EventCommitter                      = (*Effects)(nil)
	_ runs.TreeBarrierCommitter                = (*Effects)(nil)
	_ runs.WaitingCheckpointReader             = (*Effects)(nil)
	_ runs.WaitingSubtreeCancellationCommitter = (*Effects)(nil)
)

const runsegmentTracerName = "scope/scopeapp/runsegment"

// New returns the durable Run-segment effects. Every dependency needed by the
// supported write-sets is validated here; optional product capabilities remain
// explicit through ScheduleFirings, GoalRuns, and ToolResults.
func New(cfg Config) (*Effects, error) {
	required := []struct {
		name  string
		value any
	}{
		{"interrupt store", cfg.Interrupts},
		{"resume claim store", cfg.ResumeClaims},
		{"session store", cfg.Sessions},
		{"transcript store", cfg.Transcript},
		{"item replacer", cfg.ItemReplacer},
		{"tool approval store", cfg.ToolApprovals},
		{"model invocation journal", cfg.ModelInvocations},
		{"tool invocation journal", cfg.ToolInvocations},
		{"conversation store", cfg.Conversation},
		{"run store", cfg.State},
		{"run progress writer", cfg.RunProgress},
		{"executor checkpoint store", cfg.ExecutorCheckpoints},
		{"child run start store", cfg.ChildRunStarts},
		{"transactor", cfg.Tx},
	}
	for _, dependency := range required {
		if nilDependency(dependency.value) {
			return nil, fmt.Errorf("runsegment: %s is required", dependency.name)
		}
	}
	optional := []struct {
		name  string
		value any
	}{
		{"schedule firing store", cfg.ScheduleFirings},
		{"goal run recorder", cfg.GoalRuns},
		{"tool result store", cfg.ToolResults},
	}
	for _, dependency := range optional {
		if dependency.value != nil && nilDependency(dependency.value) {
			return nil, fmt.Errorf("runsegment: optional %s must not be typed nil", dependency.name)
		}
	}
	return &Effects{
		interrupts:          cfg.Interrupts,
		resumeClaims:        cfg.ResumeClaims,
		sessions:            cfg.Sessions,
		scheduleFirings:     cfg.ScheduleFirings,
		goalRuns:            cfg.GoalRuns,
		transcript:          cfg.Transcript,
		itemReplacer:        cfg.ItemReplacer,
		toolApprovals:       cfg.ToolApprovals,
		toolResults:         cfg.ToolResults,
		modelInvocations:    cfg.ModelInvocations,
		toolInvocations:     cfg.ToolInvocations,
		conversation:        cfg.Conversation,
		runState:            cfg.State,
		runProgress:         cfg.RunProgress,
		executorCheckpoints: cfg.ExecutorCheckpoints,
		childRunStarts:      cfg.ChildRunStarts,
		tx:                  cfg.Tx,
	}, nil
}

func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	kind := reflect.ValueOf(value).Kind()
	return (kind == reflect.Chan || kind == reflect.Func || kind == reflect.Interface ||
		kind == reflect.Map || kind == reflect.Pointer || kind == reflect.Slice) &&
		reflect.ValueOf(value).IsNil()
}

// ReadWaitingCheckpoint returns the exact opaque recovery point selected by
// the Application waiting-tree use case.
func (e *Effects) ReadWaitingCheckpoint(
	ctx context.Context,
	rootMemberID string,
) (runs.ExecutorCheckpoint, error) {
	checkpoint, err := e.executorCheckpoints.LoadCheckpoint(ctx, rootMemberID)
	if err != nil {
		return runs.ExecutorCheckpoint{}, fmt.Errorf("runsegment: load waiting executor checkpoint: %w", err)
	}
	return checkpoint.Clone(), nil
}
