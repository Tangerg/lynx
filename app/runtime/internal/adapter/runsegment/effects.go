// Package runsegment is the driven adapter that executes the durable side
// effects of one streamed run segment. It implements the application's
// runs.Effects port: the run pump hands it an [execution.EventCommit] per event,
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

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
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
}

// TranscriptStore is the run-segment append/upsert side of durable transcript
// persistence. Reading and destructive deletion belong to other use-cases.
type TranscriptStore interface {
	AppendItem(ctx context.Context, it transcript.Item) error
	PutRun(ctx context.Context, r transcript.Run) error
}

// RunStateWriter applies the run's mid-flight admission-state transitions inside
// the event commit (§8.3): a park suspends the run, a terminal terminalizes it —
// each in the SAME transaction as the interrupt / terminal record it must stay
// consistent with. The sqlite RunStateStore satisfies it.
type RunStateWriter interface {
	Suspend(ctx context.Context, sessionID string) error
	Terminalize(ctx context.Context, sessionID, outcome string) error
}

// Transactor runs fn inside one storage transaction; the store calls fn makes
// join it through the context. A nil Transactor degrades to a direct
// (non-transactional) call for tests / non-sqlite runtimes.
type Transactor func(ctx context.Context, fn func(context.Context) error) error

// Stores is the consumer-defined surface the Effects coordinator drives. It is
// intentionally narrower than the runtime bundle: this use-case needs only
// durable transcript/interrupt stores, a tiny session view, chat history count,
// and title generation.
type Stores interface {
	Interrupts() InterruptStore
	Session() SessionStore
	Transcript() TranscriptStore
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

// CommitEvent applies one run event's durable parts atomically (§8.3/§8.4): the
// open-interrupt record, transcript item/run projections, and the run-state
// transition, all in one transaction. The interrupt's recoverable process id is
// resolved from the live turn BEFORE the transaction opens (an in-memory lookup,
// not a DB read) and its absence fails the commit — a park with no recoverable
// process is not resumable. A terminal run's message watermark is resolved inside
// the transaction so it is consistent with the state it terminalizes.
func (e *Effects) CommitEvent(ctx context.Context, commit execution.EventCommit) error {
	var pending *interrupts.Pending
	if commit.Interrupt != nil {
		p := *commit.Interrupt
		procID, err := e.interruptProcessID(ctx, p)
		if err != nil {
			return err
		}
		p.ProcessID = procID
		pending = &p
	}
	return e.runInTx(ctx, func(ctx context.Context) error {
		if pending != nil {
			if err := e.putInterrupt(ctx, *pending); err != nil {
				return err
			}
		}
		if commit.Item != nil {
			if err := e.appendItem(ctx, *commit.Item); err != nil {
				return err
			}
		}
		if commit.Run != nil {
			if err := e.putRun(ctx, *commit.Run, commit.State == execution.StateTerminalize); err != nil {
				return err
			}
		}
		return e.applyState(ctx, commit)
	})
}

// Nudge publishes a non-durable live workspace change to subscribers.
func (e *Effects) Nudge(cwd string, paths []string) {
	if e.publish != nil && len(paths) > 0 {
		e.publish(cwd, paths)
	}
}

// Finish starts best-effort terminal maintenance off the live stream path. A
// parked run is resumable, not a boundary, so it does not snapshot or title.
func (e *Effects) Finish(ctx context.Context, fin runs.Finish) {
	if fin.Parked {
		return
	}
	if e.checkpoints != nil {
		e.startBackground(ctx, func(ctx context.Context) {
			e.snapshot(ctx, fin.SessionID, fin.RunID)
		})
	}
	if fin.OpeningUserText != "" {
		e.startBackground(ctx, func(ctx context.Context) {
			e.title(ctx, fin.SessionID, fin.OpeningUserText)
		})
	}
}

func (e *Effects) runInTx(ctx context.Context, fn func(context.Context) error) error {
	if e.tx == nil {
		return fn(ctx)
	}
	return e.tx(ctx, fn)
}

func (e *Effects) interruptProcessID(ctx context.Context, p interrupts.Pending) (string, error) {
	if e.processes == nil {
		return "", errors.New("runsegment: interrupt persistence is unavailable")
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

// appendItem is a best-effort transcript projection write: a missing store (a
// non-persisting test runtime) or a lost item is a display gap, not a run error.
func (e *Effects) appendItem(ctx context.Context, item transcript.Item) error {
	if e.stores == nil || e.stores.Transcript() == nil {
		return nil
	}
	return e.stores.Transcript().AppendItem(ctx, item)
}

// putRun upserts a transcript run, resolving the terminal message watermark
// inside the caller's transaction — the mark the rollback / fork boundary math
// truncates the chat log to. The message log is in its terminal post-maintenance
// (post-compaction) shape by the time the terminal event reaches here.
func (e *Effects) putRun(ctx context.Context, run transcript.Run, terminal bool) error {
	if e.stores == nil || e.stores.Transcript() == nil {
		return nil
	}
	if terminal && run.Mark < 0 {
		if mark, err := e.stores.MessageCount(ctx, run.SessionID); err == nil {
			run.Mark = mark
		}
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = time.Now().UTC()
	}
	return e.stores.Transcript().PutRun(ctx, run)
}

func (e *Effects) applyState(ctx context.Context, commit execution.EventCommit) error {
	if e.runState == nil {
		return nil
	}
	switch commit.State {
	case execution.StateSuspend:
		return e.runState.Suspend(ctx, commit.SessionID)
	case execution.StateTerminalize:
		return e.runState.Terminalize(ctx, commit.SessionID, commit.Outcome.String())
	default:
		return nil
	}
}

func (e *Effects) startBackground(ctx context.Context, task func(context.Context)) {
	if e.tasks != nil {
		e.tasks.Start(ctx, task)
		return
	}
	task(ctx)
}

func (e *Effects) snapshot(ctx context.Context, sessionID, runID string) {
	cwd := e.sessionCwd(ctx, sessionID)
	if cwd == "" {
		return
	}
	_ = e.checkpoints.Snapshot(ctx, sessionID, cwd, runID)
}

func (e *Effects) title(ctx context.Context, sessionID, prompt string) {
	if e.stores == nil || e.stores.Session() == nil {
		return
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return
	}
	if sess, err := e.stores.Session().Get(ctx, sessionID); err != nil || strings.TrimSpace(sess.Title) != "" {
		return
	}
	title, err := e.stores.GenerateTitle(ctx, prompt)
	if err != nil || title == "" {
		return
	}
	_ = e.stores.Session().RenameIfUntitled(ctx, sessionID, title)
}

func (e *Effects) sessionCwd(ctx context.Context, sessionID string) string {
	if e.stores == nil || e.stores.Session() == nil {
		return ""
	}
	sess, err := e.stores.Session().Get(ctx, sessionID)
	if err != nil {
		return ""
	}
	return sess.Cwd
}
