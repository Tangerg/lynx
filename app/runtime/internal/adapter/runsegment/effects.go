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
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

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

// Stores is the consumer-defined surface the Effects coordinator drives. It is
// intentionally narrower than the runtime bundle: this use-case needs only
// durable transcript/interrupt stores, a tiny session view, chat history count,
// and title generation.
type Stores interface {
	Interrupts() InterruptStore
	Session() SessionStore
	Transcript() TranscriptStore
	ToolResults() ToolResultStore
	MessageCount(ctx context.Context, sessionID string) (int, error)
	GenerateTitle(ctx context.Context, firstMessage string) (string, error)
}

// ProcessLookup resolves the agent process backing a live turn. The full
// turn.Dispatcher has many operations; runsegment needs only this one.
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
type FileChangePublisher func(cwd string, paths []string)

// Config bundles the Effects dependencies.
type Config struct {
	Stores             Stores
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
	stores      Stores
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
		stores:      cfg.Stores,
		processes:   cfg.Processes,
		runState:    cfg.RunState,
		tx:          cfg.Tx,
		checkpoints: cfg.Checkpoints,
		tasks:       cfg.Tasks,
		publish:     cfg.PublishFileChanges,
	}
}

// CommitOpening accepts one segment atomically. A fresh segment admits its Run;
// a continuation consumes the open interrupt and resumes the existing Run. The
// opening transcript projections land in that same transaction, so Start cannot
// acknowledge a segment whose durable opening is missing.
func (e *Effects) CommitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	if e.tx == nil {
		return errors.New("runsegment: transactor is unavailable")
	}
	if (opening.Admit == nil) == (opening.Resume == nil) {
		return errors.New("runsegment: opening requires exactly one admission action")
	}
	if len(opening.Events) == 0 {
		return errors.New("runsegment: opening requires a durable projection")
	}
	return e.runInTx(ctx, func(ctx context.Context) error {
		switch {
		case opening.Admit != nil:
			if e.runState == nil {
				return errors.New("runsegment: run-state persistence is unavailable")
			}
			if err := e.runState.Admit(ctx, *opening.Admit); err != nil {
				return err
			}
		case opening.Resume != nil:
			if err := e.consumeResume(ctx, *opening.Resume); err != nil {
				return err
			}
		}
		for _, commit := range opening.Events {
			if commit.Interrupt != nil || commit.State != runs.StateUnchanged {
				return errors.New("runsegment: opening commit contains a lifecycle transition")
			}
			if len(commit.Items) == 0 && commit.Run == nil {
				return errors.New("runsegment: opening commit has no durable projection")
			}
			if err := e.applyCommit(ctx, commit, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

// CommitEvent applies one run event's durable parts atomically (§8.3/§8.4): the
// open-interrupt record, transcript item/run projections, and the run-state
// transition, all in one transaction. The interrupt's recoverable process id is
// resolved from the live turn BEFORE the transaction opens (an in-memory lookup,
// not a DB read) and its absence fails the commit — a park with no recoverable
// process is not resumable. A terminal run's message watermark is resolved inside
// the transaction so it is consistent with the state it terminalizes.
func (e *Effects) CommitEvent(ctx context.Context, commit runs.EventCommit) error {
	if e.tx == nil {
		return errors.New("runsegment: transactor is unavailable")
	}
	var pending *interrupts.Pending
	if commit.Interrupt != nil {
		p := *commit.Interrupt
		procID, err := e.interruptProcessID(ctx, p)
		if err != nil {
			return e.compensateFailedCommit(ctx, commit, err)
		}
		p.ProcessID = procID
		pending = &p
	}
	err := e.runInTx(ctx, func(ctx context.Context) error { return e.applyCommit(ctx, commit, pending) })
	if err != nil {
		return e.compensateFailedCommit(ctx, commit, err)
	}
	return nil
}

const stagedToolResultCleanupTimeout = 5 * time.Second

// compensateFailedCommit removes only unbound blobs staged by the failed
// event. Cleanup is request-detached because cancellation is one of the failure
// paths; Discard's unbound predicate makes an ambiguous successful commit safe.
func (e *Effects) compensateFailedCommit(ctx context.Context, commit runs.EventCommit, commitErr error) error {
	if e.stores == nil || e.stores.ToolResults() == nil {
		return commitErr
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stagedToolResultCleanupTimeout)
	defer cancel()
	var cleanupErrs []error
	for _, item := range commit.Items {
		if item.Tool == nil || item.Tool.Offload == nil {
			continue
		}
		if err := e.stores.ToolResults().Discard(cleanupCtx, item.SessionID, *item.Tool.Offload); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("runsegment: discard staged tool result %q: %w", item.Tool.Offload.ID, err))
		}
	}
	return errors.Join(commitErr, errors.Join(cleanupErrs...))
}

func (e *Effects) applyCommit(ctx context.Context, commit runs.EventCommit, pending *interrupts.Pending) error {
	if pending != nil {
		if err := e.putInterrupt(ctx, *pending); err != nil {
			return err
		}
	}
	for _, item := range commit.Items {
		if err := e.appendItem(ctx, item); err != nil {
			return err
		}
	}
	if commit.Run != nil {
		if err := e.putRun(ctx, *commit.Run, commit.State == runs.StateTerminalize); err != nil {
			return err
		}
	}
	return e.applyState(ctx, commit)
}

func (e *Effects) consumeResume(ctx context.Context, resume execution.ResumeDraft) error {
	if e.stores == nil || e.stores.Interrupts() == nil {
		return errors.New("runsegment: interrupt persistence is unavailable")
	}
	pending, ok, err := e.stores.Interrupts().Consume(ctx, resume.RunID)
	if err != nil {
		return fmt.Errorf("runsegment: consume resume interrupt: %w", err)
	}
	if !ok {
		return errors.New("runsegment: resume interrupt is no longer open")
	}
	if pending.SessionID != resume.SessionID {
		return fmt.Errorf("runsegment: resume interrupt session mismatch: got %q want %q", pending.SessionID, resume.SessionID)
	}
	if e.runState == nil {
		return errors.New("runsegment: run-state persistence is unavailable")
	}
	if err := e.runState.Resume(ctx, resume); err != nil {
		return fmt.Errorf("runsegment: resume run state: %w", err)
	}
	return nil
}

// Nudge publishes a non-durable live workspace change to subscribers.
func (e *Effects) Nudge(cwd string, paths []string) {
	if e.publish != nil && len(paths) > 0 {
		e.publish(cwd, paths)
	}
}

// Finish establishes the terminal file boundary before returning, then starts
// title generation off the live path. The checkpoint is a sequencing fence: the
// run admission remains held by the caller until it completes, so a following
// run cannot write into the preceding run's snapshot. Title generation does not
// define the boundary and may continue asynchronously. A parked run is
// resumable, not a boundary, so it does neither.
func (e *Effects) Finish(ctx context.Context, fin runs.Finish) error {
	if fin.Parked {
		return nil
	}
	needsSnapshot := e.checkpoints != nil && fin.Cwd != ""
	needsTitle := strings.TrimSpace(fin.OpeningUserText) != ""
	if !needsSnapshot && !needsTitle {
		return nil
	}
	var errs []error
	if needsSnapshot {
		if err := observeTerminalMaintenance(ctx, fin, "checkpoint", func(ctx context.Context) error {
			return e.snapshot(ctx, fin.SessionID, fin.Cwd, fin.RunID)
		}); err != nil {
			errs = append(errs, err)
		}
	}
	if !needsTitle {
		return errors.Join(errs...)
	}
	title := func(ctx context.Context) error {
		return observeTerminalMaintenance(ctx, fin, "title", func(ctx context.Context) error {
			return e.title(ctx, fin.SessionID, fin.OpeningUserText)
		})
	}
	if e.tasks == nil {
		return errors.Join(append(errs, title(ctx))...)
	}
	if !e.tasks.Start(ctx, func(ctx context.Context) { _ = title(ctx) }) {
		rejected := fmt.Errorf("runsegment: terminal maintenance for run %q was rejected during shutdown", fin.RunID)
		errs = append(errs, observeTerminalMaintenance(ctx, fin, "title", func(context.Context) error { return rejected }))
	}
	return errors.Join(errs...)
}

func observeTerminalMaintenance(ctx context.Context, fin runs.Finish, operation string, maintenance func(context.Context) error) error {
	ctx, span := otel.Tracer(runsegmentTracerName).Start(ctx, "run terminal maintenance",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("run.id", fin.RunID),
			attribute.String("gen_ai.conversation.id", fin.SessionID),
			attribute.String("maintenance.operation", operation),
		),
	)
	defer span.End()
	err := maintenance(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (e *Effects) runInTx(ctx context.Context, fn func(context.Context) error) error {
	return e.tx(ctx, fn)
}

func (e *Effects) interruptProcessID(ctx context.Context, p interrupts.Pending) (string, error) {
	if e.processes == nil {
		return "", errors.New("runsegment: process lookup is unavailable")
	}
	// Rebuild the executor's turn handle from the persisted coordinates — the
	// dispatcher keys the live turn by session + turn id, and the domain record
	// carries both, so runsegment needs no adapter handle in the commit value.
	procID, err := e.processes.ProcessID(ctx, turn.TurnHandle{SessionID: p.SessionID, TurnID: p.TurnID})
	if err != nil {
		return "", fmt.Errorf("runsegment: resolve interrupt process: %w", err)
	}
	if procID == "" {
		return "", errors.New("runsegment: interrupt process id is empty")
	}
	return procID, nil
}

func (e *Effects) putInterrupt(ctx context.Context, p interrupts.Pending) error {
	if e.stores == nil || e.stores.Interrupts() == nil {
		return errors.New("runsegment: interrupt persistence is unavailable")
	}
	if err := e.stores.Interrupts().Put(ctx, p); err != nil {
		return fmt.Errorf("runsegment: persist interrupt: %w", err)
	}
	return nil
}

func (e *Effects) appendItem(ctx context.Context, item transcript.Item) error {
	if e.stores == nil || e.stores.Transcript() == nil {
		return errors.New("runsegment: transcript persistence is unavailable")
	}
	if err := e.stores.Transcript().AppendItem(ctx, item); err != nil {
		return err
	}
	if item.Tool == nil || item.Tool.Offload == nil {
		return nil
	}
	if item.Tool.Result == nil {
		return errors.New("runsegment: offloaded tool result is absent")
	}
	preview, ok := item.Tool.Result.String()
	if !ok {
		return errors.New("runsegment: offloaded tool result has no preview string")
	}
	if e.stores.ToolResults() == nil {
		return errors.New("runsegment: tool-result persistence is unavailable")
	}
	if err := e.stores.ToolResults().Bind(ctx, item.SessionID, item.ID, preview, *item.Tool.Offload); err != nil {
		return fmt.Errorf("runsegment: bind offloaded tool result: %w", err)
	}
	return nil
}

// putRun upserts a transcript run, resolving the terminal message watermark
// inside the caller's transaction — the mark the rollback / fork boundary math
// truncates the chat log to. The message log is in its terminal post-maintenance
// (post-compaction) shape by the time the terminal event reaches here.
func (e *Effects) putRun(ctx context.Context, run transcript.Run, terminal bool) error {
	if e.stores == nil || e.stores.Transcript() == nil {
		return errors.New("runsegment: transcript persistence is unavailable")
	}
	if terminal && run.MessageMark < 0 {
		mark, err := e.stores.MessageCount(ctx, run.SessionID)
		if err != nil {
			return fmt.Errorf("runsegment: resolve terminal message watermark: %w", err)
		}
		run.MessageMark = mark
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = time.Now().UTC()
	}
	return e.stores.Transcript().PutRun(ctx, run)
}

func (e *Effects) applyState(ctx context.Context, commit runs.EventCommit) error {
	if commit.State == runs.StateUnchanged {
		return nil
	}
	if e.runState == nil {
		return errors.New("runsegment: run-state persistence is unavailable")
	}
	switch commit.State {
	case runs.StateSuspend:
		return e.runState.Suspend(ctx, commit.SessionID, commit.RunID)
	case runs.StateTerminalize:
		return e.runState.Terminalize(ctx, commit.SessionID, commit.RunID, commit.Outcome)
	default:
		return fmt.Errorf("runsegment: unknown run state change %d", commit.State)
	}
}

func (e *Effects) snapshot(ctx context.Context, sessionID, cwd, runID string) error {
	if err := e.checkpoints.Snapshot(ctx, sessionID, cwd, runID); err != nil {
		return fmt.Errorf("runsegment: snapshot workspace for run %q: %w", runID, err)
	}
	return nil
}

func (e *Effects) title(ctx context.Context, sessionID, prompt string) error {
	if e.stores == nil || e.stores.Session() == nil {
		return errors.New("runsegment: session persistence is unavailable for title generation")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}
	sess, err := e.stores.Session().Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("runsegment: load session %q for title generation: %w", sessionID, err)
	}
	if strings.TrimSpace(sess.Title) != "" {
		return nil
	}
	title, err := e.stores.GenerateTitle(ctx, prompt)
	if err != nil {
		return fmt.Errorf("runsegment: generate title for session %q: %w", sessionID, err)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("runsegment: generated title for session %q is empty", sessionID)
	}
	if err := e.stores.Session().RenameIfUntitled(ctx, sessionID, title); err != nil {
		return fmt.Errorf("runsegment: rename untitled session %q: %w", sessionID, err)
	}
	return nil
}
