// Package runsegment is the driven adapter that executes the durable side
// effects of one streamed run segment. It implements the application's
// runs.Effects port: the run pump hands it a [runs.EventCommit] per event,
// which it applies ATOMICALLY — the open-interrupt record, transcript
// projections, and the run-state transition land in one transaction (§8.3/§8.4),
// so a crash never leaves a parked run with no admission mark or a terminal
// transcript with a still-running row. It also runs the non-durable live
// workspace nudge and terminal boundary maintenance (checkpoint snapshot,
// title). The fields only the runtime can resolve — an interrupt's process id
// from the live turn, a terminal run's message watermark — it fills in itself.
package runsegment

import (
	"context"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// SessionStore is the run-segment side-effect view of session persistence.
// Terminal maintenance only needs the session's cwd/title and the atomic
// untitled-title update; it should not depend on the full domain Store.
type SessionStore interface {
	Get(ctx context.Context, id string) (session.Session, error)
	RenameIfUntitled(ctx context.Context, id, title string) error
}

// InterruptStore is the run-segment write side of the open-interrupt registry.
// A stream segment only records newly-opened interrupts; claim/resume/delete
// belongs to lifecycle.
type InterruptStore interface {
	Put(ctx context.Context, p interrupts.Pending) error
	Consume(ctx context.Context, runID string) (interrupts.Pending, bool, error)
}

// TranscriptStore is the run-segment append/upsert side of durable transcript
// persistence. Reading and destructive deletion belong to other use-cases.
type TranscriptStore interface {
	AppendItem(ctx context.Context, it transcript.Item) error
	PutRun(ctx context.Context, r transcript.Run) error
}

type ToolResultStore interface {
	Bind(ctx context.Context, sessionID, itemID, preview string, ref offload.Ref) error
	Discard(ctx context.Context, sessionID string, ref offload.Ref) error
}

// RunStateWriter applies the run's mid-flight admission-state transitions inside
// the event commit (§8.3): a park suspends the run, a terminal terminalizes it —
// each in the SAME transaction as the interrupt / terminal record it must stay
// consistent with. The sqlite RunStateStore satisfies it.
type RunStateWriter interface {
	Admit(ctx context.Context, draft execution.RunDraft) error
	Resume(ctx context.Context, draft execution.ResumeDraft) error
	Suspend(ctx context.Context, sessionID, runID string) error
	Terminalize(ctx context.Context, sessionID, runID string, o execution.Outcome) error
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

// ProcessLookup resolves the agent process backing a live turn. The concrete
// turn dispatcher has many operations; runsegment needs only this one.
type ProcessLookup interface {
	ProcessID(ctx context.Context, handle turn.TurnHandle) (string, error)
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
// mutation. It is deliberately path-only: the protocol adapter owns the wire
// WorkspaceEvent shape.
type FileChangePublisher func(runs.FileChange)

// Config bundles the Effects dependencies.
type Config struct {
	Interrupts         InterruptStore
	Sessions           SessionStore
	Transcript         TranscriptStore
	ToolResults        ToolResultStore
	Messages           MessageCounter
	Titles             TitleGenerator
	Processes          ProcessLookup
	RunState           RunStateWriter
	Tx                 Transactor
	Checkpoints        Checkpoints
	Tasks              TaskLauncher
	PublishFileChanges FileChangePublisher
}

// Effects coordinates run-segment side effects. It is stateless beyond its
// dependencies and safe to share.
type Effects struct {
	interrupts  InterruptStore
	sessions    SessionStore
	transcript  TranscriptStore
	toolResults ToolResultStore
	messages    MessageCounter
	titles      TitleGenerator
	processes   ProcessLookup
	runState    RunStateWriter
	tx          Transactor
	checkpoints Checkpoints
	tasks       TaskLauncher
	publish     FileChangePublisher
}

var _ runs.Effects = (*Effects)(nil)

const runsegmentTracerName = "lynx/lyra/runsegment"

// New returns an Effects coordinator.
func New(cfg Config) *Effects {
	return &Effects{
		interrupts:  cfg.Interrupts,
		sessions:    cfg.Sessions,
		transcript:  cfg.Transcript,
		toolResults: cfg.ToolResults,
		messages:    cfg.Messages,
		titles:      cfg.Titles,
		processes:   cfg.Processes,
		runState:    cfg.RunState,
		tx:          cfg.Tx,
		checkpoints: cfg.Checkpoints,
		tasks:       cfg.Tasks,
		publish:     cfg.PublishFileChanges,
	}
}

// Nudge publishes a non-durable live workspace change to subscribers.
func (e *Effects) Nudge(cwd string, paths []string) {
	if e.publish != nil && len(paths) > 0 {
		e.publish(runs.FileChange{Cwd: cwd, Paths: slices.Clone(paths)})
	}
}
