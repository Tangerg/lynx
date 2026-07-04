// Package runsegment owns the non-wire side effects of one streamed run
// segment: durable transcript writes, open-interrupt records, workspace change
// nudges, and terminal best-effort maintenance.
package runsegment

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Stores is the consumer-defined surface the Effects coordinator drives. It is
// intentionally narrower than the runtime bundle: this use-case needs only
// durable transcript/interrupt/session stores, chat-memory count, and title
// generation.
type Stores interface {
	Interrupts() interrupts.Store
	Session() session.Service
	Transcript() transcript.Store
	MessageCount(ctx context.Context, sessionID string) (int, error)
	GenerateTitle(ctx context.Context, firstMessage string) (string, error)
}

// ProcessLookup resolves the agent process backing a live turn. The full
// turn.Service has many operations; runsegment needs only this one.
type ProcessLookup interface {
	ProcessID(ctx context.Context, handle turn.TurnHandle) (string, error)
}

// Checkpoints anchors the working tree at a terminal run boundary. Implemented
// by the workspace adapter; defined here so the kernel depends on the behavior,
// not the adapter package.
type Checkpoints interface {
	Snapshot(ctx context.Context, sessionID, cwd, runID string) error
}

// FileChangePublisher nudges live workspace subscribers after a tool-owned file
// mutation. It is deliberately path-only: the protocol adapter owns the wire
// WorkspaceEvent shape.
type FileChangePublisher func(cwd string, paths []string)

// Config bundles the Effects dependencies.
type Config struct {
	Stores             Stores
	Processes          ProcessLookup
	Checkpoints        Checkpoints
	PublishFileChanges FileChangePublisher
}

// Effects coordinates run-segment side effects. It is stateless beyond its
// dependencies and safe to share.
type Effects struct {
	stores      Stores
	processes   ProcessLookup
	checkpoints Checkpoints
	publish     FileChangePublisher
}

// New returns an Effects coordinator.
func New(cfg Config) *Effects {
	return &Effects{
		stores:      cfg.Stores,
		processes:   cfg.Processes,
		checkpoints: cfg.Checkpoints,
		publish:     cfg.PublishFileChanges,
	}
}

// Event is the delivery-neutral side-effect payload for one stream event. The
// protocol adapter builds it while translating wire shapes; Effects only sees
// domain records and opaque JSON blobs.
type Event struct {
	Interrupt    *Interrupt
	Item         *transcript.Item
	Run          *RunRecord
	FilesChanged *FilesChanged
}

// Interrupt is the durable-resume record opened when a run parks on HITL.
type Interrupt struct {
	RunID        string
	Handle       turn.TurnHandle
	Provider     string
	Model        string
	Payload      []byte
	DrainedTools []interrupts.DrainedTool
}

// RunRecord is a transcript run upsert. Terminal records get their chat-memory
// watermark resolved by Effects immediately before PutRun.
type RunRecord struct {
	Run      transcript.Run
	Terminal bool
}

// FilesChanged is the workspace live-update nudge derived from a completed
// file-mutating tool call.
type FilesChanged struct {
	Cwd   string
	Paths []string
}

// Finish describes the terminal run-boundary maintenance that is safe to run
// after the live stream has been closed.
type Finish struct {
	SessionID       string
	RunID           string
	Parked          bool
	OpeningUserText string
}

// BeforeLive runs side effects that must complete before the terminal event is
// visible to subscribers. Today that means only the interrupt record: a client
// may call runs.resume as soon as it observes run.finished{interrupt}.
func (e *Effects) BeforeLive(ctx context.Context, ev Event) {
	if ev.Interrupt != nil {
		e.recordInterrupt(ctx, *ev.Interrupt)
	}
}

// AfterLive runs side effects that must not block live delivery: durable
// transcript writes and workspace change nudges. The caller should pass a
// cancel-decoupled context for terminal events that must survive run cancel.
func (e *Effects) AfterLive(ctx context.Context, ev Event) {
	if ev.Item != nil {
		e.recordItem(ctx, *ev.Item)
	}
	if ev.Run != nil {
		e.recordRun(ctx, *ev.Run)
	}
	if ev.FilesChanged != nil && e.publish != nil && len(ev.FilesChanged.Paths) > 0 {
		e.publish(ev.FilesChanged.Cwd, ev.FilesChanged.Paths)
	}
}

// Finish starts best-effort terminal maintenance off the live stream path. A
// parked run is resumable, not a boundary, so it does not snapshot or title.
func (e *Effects) Finish(ctx context.Context, fin Finish) {
	if fin.Parked {
		return
	}
	ctx = context.WithoutCancel(ctx)
	if e.checkpoints != nil {
		go e.snapshot(ctx, fin.SessionID, fin.RunID)
	}
	if fin.OpeningUserText != "" {
		go e.title(ctx, fin.SessionID, fin.OpeningUserText)
	}
}

func (e *Effects) recordInterrupt(ctx context.Context, in Interrupt) {
	if e.stores == nil || e.stores.Interrupts() == nil || e.processes == nil {
		return
	}
	processID, err := e.processes.ProcessID(ctx, in.Handle)
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
	if err := e.stores.Interrupts().Put(ctx, interrupts.Pending{
		ParentRunID:  in.RunID,
		SessionID:    in.Handle.SessionID,
		TurnID:       in.Handle.TurnID,
		ProcessID:    processID,
		Provider:     in.Provider,
		Model:        in.Model,
		Interrupts:   in.Payload,
		DrainedTools: in.DrainedTools,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}

func (e *Effects) recordItem(ctx context.Context, item transcript.Item) {
	if e.stores == nil || e.stores.Transcript() == nil {
		return
	}
	if err := e.stores.Transcript().AppendItem(ctx, item); err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}

func (e *Effects) recordRun(ctx context.Context, rec RunRecord) {
	if e.stores == nil || e.stores.Transcript() == nil {
		return
	}
	if rec.Terminal {
		mark, err := e.stores.MessageCount(ctx, rec.Run.SessionID)
		if err != nil {
			mark = -1
		}
		rec.Run.Mark = mark
	}
	if rec.Run.UpdatedAt.IsZero() {
		rec.Run.UpdatedAt = time.Now().UTC()
	}
	if err := e.stores.Transcript().PutRun(ctx, rec.Run); err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
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
