// Package runrecovery applies the boot-recovery write-set selected by the
// application. It coordinates persistence mechanics only; recovery policy and
// executor-checkpoint interpretation stay outside this adapter.
package runrecovery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type RunStore interface {
	ListNonTerminalRuns(ctx context.Context) ([]run.Run, error)
	RecoverLost(ctx context.Context, run run.Run) error
}

type SessionStore interface {
	Get(ctx context.Context, sessionID string) (session.Session, error)
}

type InterruptStore interface {
	List(ctx context.Context, sessionID string) ([]runs.Pending, error)
	Delete(ctx context.Context, sessionID, rootRunID string) error
}

type TranscriptStore interface {
	List(ctx context.Context, sessionID string) ([]transcript.Item, error)
	ReplaceItem(ctx context.Context, expected transcript.Item, replacement transcript.Item) error
}

type ConversationStore interface {
	Read(ctx context.Context, sessionID string) ([]corechat.Message, error)
	Write(ctx context.Context, sessionID string, messages ...corechat.Message) error
	Count(ctx context.Context, sessionID string) (int, error)
}

type GoalRunRecorder interface {
	RecordRun(ctx context.Context, record goal.RunRecord) error
}

type ExecutorCheckpointStore interface {
	LoadCheckpoint(ctx context.Context, rootMemberID string) (runs.ExecutorCheckpoint, error)
	DeleteSessionCheckpoints(ctx context.Context, sessionID string) error
}

type ModelInvocationStore interface {
	ListStartedModelInvocations(
		ctx context.Context,
		yield func(sessionID, runID, segmentID, callID string, startedAt time.Time) error,
	) error
	MarkModelInvocationUnknown(
		ctx context.Context,
		sessionID, runID, segmentID, callID string,
		startedAt, finishedAt time.Time,
	) error
}

type ToolInvocationStore interface {
	ListStartedToolInvocations(
		ctx context.Context,
		yield func(sessionID, runID, segmentID, callID, itemID string, startedAt time.Time) error,
	) error
	MarkToolInvocationIncomplete(
		ctx context.Context,
		sessionID, runID, segmentID, callID, itemID string,
		startedAt, finishedAt time.Time,
	) error
}

// ChildRunStartReservations owns the process-scoped callback ledger behind
// child admission. Its rows are not recovery facts: callbacks die with the
// process, so reconciliation retires the claimed Session's abandoned ledger in
// the same transaction as the public Run repair.
type ChildRunStartReservations interface {
	DeleteSession(ctx context.Context, sessionID string) error
}

type Transactor func(ctx context.Context, fn func(context.Context) error) error

type Config struct {
	Sessions            SessionStore
	Runs                RunStore
	Interrupts          InterruptStore
	Transcript          TranscriptStore
	Messages            ConversationStore
	GoalRuns            GoalRunRecorder
	ExecutorCheckpoints ExecutorCheckpointStore
	ModelInvocations    ModelInvocationStore
	ToolInvocations     ToolInvocationStore
	ChildRunStarts      ChildRunStartReservations
	Tx                  Transactor
}

// Persistence implements the recovery use case's durable fact and atomic
// commit port.
type Persistence struct {
	sessions            SessionStore
	runs                RunStore
	interrupts          InterruptStore
	transcript          TranscriptStore
	messages            ConversationStore
	goalRuns            GoalRunRecorder
	executorCheckpoints ExecutorCheckpointStore
	modelInvocations    ModelInvocationStore
	toolInvocations     ToolInvocationStore
	childRunStarts      ChildRunStartReservations
	tx                  Transactor
}

func New(config Config) (*Persistence, error) {
	switch {
	case config.Sessions == nil:
		return nil, errors.New("runrecovery: Session store is required")
	case config.Runs == nil:
		return nil, errors.New("runrecovery: Run store is required")
	case config.Interrupts == nil:
		return nil, errors.New("runrecovery: interrupt store is required")
	case config.Transcript == nil:
		return nil, errors.New("runrecovery: transcript store is required")
	case config.Messages == nil:
		return nil, errors.New("runrecovery: message counter is required")
	case config.ExecutorCheckpoints == nil:
		return nil, errors.New("runrecovery: executor checkpoint store is required")
	case config.ModelInvocations == nil:
		return nil, errors.New("runrecovery: model invocation store is required")
	case config.ToolInvocations == nil:
		return nil, errors.New("runrecovery: Tool invocation store is required")
	case config.ChildRunStarts == nil:
		return nil, errors.New("runrecovery: child Run start reservation store is required")
	case config.Tx == nil:
		return nil, errors.New("runrecovery: transactor is required")
	default:
		return &Persistence{
			sessions:            config.Sessions,
			runs:                config.Runs,
			interrupts:          config.Interrupts,
			transcript:          config.Transcript,
			messages:            config.Messages,
			goalRuns:            config.GoalRuns,
			executorCheckpoints: config.ExecutorCheckpoints,
			modelInvocations:    config.ModelInvocations,
			toolInvocations:     config.ToolInvocations,
			childRunStarts:      config.ChildRunStarts,
			tx:                  config.Tx,
		}, nil
	}
}

func (p *Persistence) SessionByID(ctx context.Context, sessionID string) (session.Session, error) {
	return p.sessions.Get(ctx, sessionID)
}

func (p *Persistence) ListNonTerminalRuns(ctx context.Context) ([]run.Run, error) {
	return p.runs.ListNonTerminalRuns(ctx)
}

func (p *Persistence) ListPendingInterrupts(ctx context.Context) ([]runs.Pending, error) {
	return p.interrupts.List(ctx, "")
}

func (p *Persistence) ListOpenModelInvocations(ctx context.Context) ([]runs.OpenModelInvocation, error) {
	var invocations []runs.OpenModelInvocation
	err := p.modelInvocations.ListStartedModelInvocations(
		ctx,
		func(sessionID, runID, segmentID, callID string, startedAt time.Time) error {
			invocations = append(invocations, runs.OpenModelInvocation{
				SessionID: sessionID, RunID: runID, SegmentID: segmentID,
				CallID: callID, StartedAt: startedAt,
			})
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("runrecovery: list open model invocations: %w", err)
	}
	return invocations, nil
}

func (p *Persistence) ListOpenToolInvocations(ctx context.Context) ([]runs.OpenToolInvocation, error) {
	var invocations []runs.OpenToolInvocation
	err := p.toolInvocations.ListStartedToolInvocations(
		ctx,
		func(sessionID, runID, segmentID, callID, itemID string, startedAt time.Time) error {
			invocations = append(invocations, runs.OpenToolInvocation{
				SessionID: sessionID, RunID: runID, SegmentID: segmentID,
				CallID: callID, ItemID: itemID, StartedAt: startedAt,
			})
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("runrecovery: list open Tool invocations: %w", err)
	}
	return invocations, nil
}

func (p *Persistence) ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, error) {
	return p.transcript.List(ctx, sessionID)
}

func (p *Persistence) CountMessages(ctx context.Context, sessionID string) (int, error) {
	return p.messages.Count(ctx, sessionID)
}

func (p *Persistence) ReadMessages(ctx context.Context, sessionID string) ([]corechat.Message, error) {
	return p.messages.Read(ctx, sessionID)
}

func (p *Persistence) LoadExecutorCheckpoint(
	ctx context.Context,
	rootMemberID string,
) (runs.ExecutorCheckpoint, error) {
	return p.executorCheckpoints.LoadCheckpoint(ctx, rootMemberID)
}

// CommitRecovery applies the application-produced write-set in one storage
// transaction. The adapter deliberately does not reinterpret ownership or
// resumability; it only preserves the ordering encoded in the plan.
func (p *Persistence) CommitRecovery(ctx context.Context, commit runs.RecoveryCommit) error {
	if err := commit.Validate(); err != nil {
		return fmt.Errorf("runrecovery: invalid recovery commit: %w", err)
	}
	return p.tx(ctx, func(ctx context.Context) error {
		for _, invocation := range commit.ModelInvocations {
			if err := p.modelInvocations.MarkModelInvocationUnknown(
				ctx,
				invocation.SessionID,
				invocation.RunID,
				invocation.SegmentID,
				invocation.CallID,
				invocation.StartedAt,
				invocation.FinishedAt,
			); err != nil {
				return fmt.Errorf(
					"runrecovery: mark model invocation %q unknown: %w",
					invocation.CallID,
					err,
				)
			}
		}
		for _, invocation := range commit.ToolInvocations {
			if err := p.toolInvocations.MarkToolInvocationIncomplete(
				ctx,
				invocation.SessionID,
				invocation.RunID,
				invocation.SegmentID,
				invocation.CallID,
				invocation.ItemID,
				invocation.StartedAt,
				invocation.FinishedAt,
			); err != nil {
				return fmt.Errorf(
					"runrecovery: mark Tool invocation %q incomplete: %w",
					invocation.CallID,
					err,
				)
			}
		}
		for _, transition := range commit.ConversationTransitions {
			count, err := p.messages.Count(ctx, transition.SessionID)
			if err != nil {
				return fmt.Errorf(
					"runrecovery: count conversation for root Run %q: %w",
					transition.RootRunID,
					err,
				)
			}
			if count != transition.ExpectedCount {
				return fmt.Errorf(
					"runrecovery: conversation for root Run %q moved from %d to %d messages",
					transition.RootRunID,
					transition.ExpectedCount,
					count,
				)
			}
			if err := p.messages.Write(ctx, transition.SessionID, transition.Messages...); err != nil {
				return fmt.Errorf(
					"runrecovery: close conversation for root Run %q: %w",
					transition.RootRunID,
					err,
				)
			}
		}
		for _, replacement := range commit.ItemReplacements {
			if err := p.transcript.ReplaceItem(ctx, replacement.Expected, replacement.Replacement); err != nil {
				return fmt.Errorf("runrecovery: replace transcript Item %q: %w", replacement.Expected.ID(), err)
			}
		}
		for _, lost := range commit.LostRuns {
			if err := p.runs.RecoverLost(ctx, lost); err != nil {
				return fmt.Errorf("runrecovery: recover lost Run %q: %w", lost.ID(), err)
			}
		}
		for _, record := range commit.GoalRuns {
			if p.goalRuns == nil {
				return errors.New("runrecovery: Goal Run store is unavailable for a Goal-owned lost Run")
			}
			if err := p.goalRuns.RecordRun(ctx, record); err != nil {
				return fmt.Errorf("runrecovery: record Goal Run for Run %q: %w", record.RunID, err)
			}
		}
		for _, owner := range commit.DeleteInterrupts {
			if err := p.interrupts.Delete(ctx, owner.SessionID, owner.RootRunID); err != nil {
				return fmt.Errorf("runrecovery: delete interrupt for root Run %q: %w", owner.RootRunID, err)
			}
		}
		for _, sessionID := range commit.DeleteCheckpointSessionIDs {
			if err := p.executorCheckpoints.DeleteSessionCheckpoints(ctx, sessionID); err != nil {
				return fmt.Errorf("runrecovery: delete executor checkpoints for Session %q: %w", sessionID, err)
			}
		}
		for _, sessionID := range commit.RecoveredSessionIDs {
			if err := p.childRunStarts.DeleteSession(ctx, sessionID); err != nil {
				return fmt.Errorf(
					"runrecovery: delete child Run start reservations for Session %q: %w",
					sessionID,
					err,
				)
			}
		}
		return nil
	})
}

var _ runs.RecoveryStore = (*Persistence)(nil)
