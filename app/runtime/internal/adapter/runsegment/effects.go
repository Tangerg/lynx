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
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// SessionStore is the run-segment side-effect view of session persistence.
// Opening and terminal maintenance only need the session's cwd, accepted-model
// fact, and atomic untitled-title update; it should not depend on the full
// domain Store.
type SessionStore interface {
	Get(ctx context.Context, id string) (session.Session, error)
	Ensure(ctx context.Context, sess session.Session) (session.Session, error)
	SetModel(ctx context.Context, id, model string) error
	RenameIfUntitled(ctx context.Context, id, title string) error
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
// sqlite RunStore satisfies it.
type RunWriter interface {
	Admit(ctx context.Context, draft run.Draft) error
	Resume(ctx context.Context, sessionID string, draft run.ResumeDraft, resumedAt time.Time) error
	Suspend(ctx context.Context, run transcript.Run) error
	Terminalize(ctx context.Context, run transcript.Run) error
}

// RunMetricsWriter updates only cumulative consumption for one exact active
// segment. Keeping it separate from lifecycle writing lets consumers depend on
// the narrower behavior they actually exercise.
type RunMetricsWriter interface {
	UpdateMetrics(
		ctx context.Context,
		sessionID string,
		runID string,
		segmentID string,
		metrics transcript.RunMetrics,
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
}

// Transactor runs fn inside one storage transaction: every store call made by
// fn joins that transaction through the context. Durable commits reject a nil
// transactor rather than silently weakening atomicity.
type Transactor func(ctx context.Context, fn func(context.Context) error) error

// MessageCounter resolves the conversation watermark a terminal run records.
type MessageCounter interface {
	Count(ctx context.Context, sessionID string) (int, error)
}

// TitleGenerator derives an initial session title from its opening request.
type TitleGenerator interface {
	Generate(ctx context.Context, firstMessage string) (string, error)
}

// Checkpoints anchors the working tree at a terminal run boundary. Implemented
// by the workspace adapter; defined here so the kernel depends on the behavior,
// not the adapter package.
type Checkpoints interface {
	Snapshot(ctx context.Context, sessionID, cwd, runID string) error
}

// TaskLauncher starts request-detached work owned by its component lifecycle.
type TaskLauncher interface {
	Start(parent context.Context, task func(context.Context)) bool
}

// FileChangePublisher nudges live workspace subscribers after a tool-owned file
// mutation. It is deliberately path-only so the persistence effect does not
// acquire event-presentation responsibilities.
type FileChangePublisher func(workspaceapp.FileChangeNotice)

// Config bundles the Effects dependencies.
type Config struct {
	Interrupts          InterruptStore
	ResumeClaims        ResumeClaimStore
	Sessions            SessionStore
	ScheduleFirings     ScheduleFiringStore
	GoalRuns            GoalRunRecorder
	Transcript          TranscriptStore
	ItemReplacer        ItemReplacer
	ToolResults         ToolResultStore
	ModelInvocations    ModelInvocationJournal
	ToolInvocations     ToolInvocationJournal
	Messages            MessageCounter
	Titles              TitleGenerator
	State               RunWriter
	RunMetrics          RunMetricsWriter
	ExecutorCheckpoints ExecutorCheckpointStore
	ChildRunStarts      ChildRunStartReservationStore
	Tx                  Transactor
	Checkpoints         Checkpoints
	Tasks               TaskLauncher
	PublishFileChanges  FileChangePublisher
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
	toolResults         ToolResultStore
	modelInvocations    ModelInvocationJournal
	toolInvocations     ToolInvocationJournal
	messages            MessageCounter
	titles              TitleGenerator
	runState            RunWriter
	runMetrics          RunMetricsWriter
	executorCheckpoints ExecutorCheckpointStore
	childRunStarts      ChildRunStartReservationStore
	tx                  Transactor
	checkpoints         Checkpoints
	tasks               TaskLauncher
	publish             FileChangePublisher
}

var (
	_ runs.OpeningCommitter                    = (*Effects)(nil)
	_ runs.ChildRunStartCommitter              = (*Effects)(nil)
	_ runs.ResumeClaimCommitter                = (*Effects)(nil)
	_ runs.EventCommitter                      = (*Effects)(nil)
	_ runs.TreeBarrierCommitter                = (*Effects)(nil)
	_ runs.WaitingCheckpointReader             = (*Effects)(nil)
	_ runs.WaitingSubtreeCancellationCommitter = (*Effects)(nil)
	_ runs.WorkspaceChangeNotifier             = (*Effects)(nil)
	_ runs.SegmentFinalizer                    = (*Effects)(nil)
)

const runsegmentTracerName = "lynx/lyra/runsegment"

// New returns an Effects coordinator.
func New(cfg Config) *Effects {
	return &Effects{
		interrupts:          cfg.Interrupts,
		resumeClaims:        cfg.ResumeClaims,
		sessions:            cfg.Sessions,
		scheduleFirings:     cfg.ScheduleFirings,
		goalRuns:            cfg.GoalRuns,
		transcript:          cfg.Transcript,
		itemReplacer:        cfg.ItemReplacer,
		toolResults:         cfg.ToolResults,
		modelInvocations:    cfg.ModelInvocations,
		toolInvocations:     cfg.ToolInvocations,
		messages:            cfg.Messages,
		titles:              cfg.Titles,
		runState:            cfg.State,
		runMetrics:          cfg.RunMetrics,
		executorCheckpoints: cfg.ExecutorCheckpoints,
		childRunStarts:      cfg.ChildRunStarts,
		tx:                  cfg.Tx,
		checkpoints:         cfg.Checkpoints,
		tasks:               cfg.Tasks,
		publish:             cfg.PublishFileChanges,
	}
}

// ReadWaitingCheckpoint returns the exact opaque recovery point selected by
// the Application waiting-tree use case.
func (e *Effects) ReadWaitingCheckpoint(
	ctx context.Context,
	rootMemberID string,
) (runs.ExecutorCheckpoint, error) {
	if e.executorCheckpoints == nil {
		return runs.ExecutorCheckpoint{}, errors.New("runsegment: executor checkpoint persistence is unavailable")
	}
	checkpoint, err := e.executorCheckpoints.LoadCheckpoint(ctx, rootMemberID)
	if err != nil {
		return runs.ExecutorCheckpoint{}, fmt.Errorf("runsegment: load waiting executor checkpoint: %w", err)
	}
	return checkpoint.Clone(), nil
}

// Nudge publishes a non-durable live workspace change to subscribers.
func (e *Effects) Nudge(cwd string, paths []string) {
	if e.publish != nil && len(paths) > 0 {
		e.publish(workspaceapp.FileChangeNotice{CWD: cwd, Paths: slices.Clone(paths)})
	}
}
