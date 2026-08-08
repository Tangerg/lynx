// Package runrecovery applies the boot-recovery write-set selected by the
// application. It coordinates persistence mechanics only; recovery policy and
// executor-checkpoint interpretation stay outside this adapter.
package runrecovery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type RunStore interface {
	ListNonTerminalRuns(ctx context.Context) ([]transcript.Run, error)
	RecoverLost(ctx context.Context, run transcript.Run) error
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

type MessageCounter interface {
	Count(ctx context.Context, sessionID string) (int, error)
}

type GoalRunRecorder interface {
	RecordRun(ctx context.Context, record goal.RunRecord) error
}

type ExecutorCheckpointStore interface {
	DeleteUnownedCheckpoints(ctx context.Context, keepRootIDs []string) error
}

type Transactor func(ctx context.Context, fn func(context.Context) error) error

type Config struct {
	Sessions            SessionStore
	Runs                RunStore
	Interrupts          InterruptStore
	Transcript          TranscriptStore
	Messages            MessageCounter
	GoalRuns            GoalRunRecorder
	ExecutorCheckpoints ExecutorCheckpointStore
	Tx                  Transactor
}

// Persistence implements the recovery use case's durable fact and atomic
// commit port.
type Persistence struct {
	sessions            SessionStore
	runs                RunStore
	interrupts          InterruptStore
	transcript          TranscriptStore
	messages            MessageCounter
	goalRuns            GoalRunRecorder
	executorCheckpoints ExecutorCheckpointStore
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
			tx:                  config.Tx,
		}, nil
	}
}

func (p *Persistence) SessionByID(ctx context.Context, sessionID string) (session.Session, error) {
	return p.sessions.Get(ctx, sessionID)
}

func (p *Persistence) ListNonTerminalRuns(ctx context.Context) ([]transcript.Run, error) {
	return p.runs.ListNonTerminalRuns(ctx)
}

func (p *Persistence) ListPendingInterrupts(ctx context.Context) ([]runs.Pending, error) {
	return p.interrupts.List(ctx, "")
}

func (p *Persistence) ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, error) {
	return p.transcript.List(ctx, sessionID)
}

func (p *Persistence) CountMessages(ctx context.Context, sessionID string) (int, error) {
	return p.messages.Count(ctx, sessionID)
}

// CommitRecovery applies the application-produced write-set in one storage
// transaction. The adapter deliberately does not reinterpret ownership or
// resumability; it only preserves the ordering encoded in the plan.
func (p *Persistence) CommitRecovery(ctx context.Context, commit runs.RecoveryCommit) error {
	if err := commit.Validate(); err != nil {
		return fmt.Errorf("runrecovery: invalid recovery commit: %w", err)
	}
	return p.tx(ctx, func(ctx context.Context) error {
		for _, replacement := range commit.ItemReplacements {
			if err := p.transcript.ReplaceItem(ctx, replacement.Expected, replacement.Replacement); err != nil {
				return fmt.Errorf("runrecovery: replace transcript Item %q: %w", replacement.Expected.ID, err)
			}
		}
		for _, lost := range commit.LostRuns {
			if err := p.runs.RecoverLost(ctx, lost); err != nil {
				return fmt.Errorf("runrecovery: recover lost Run %q: %w", lost.ID, err)
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
		for _, pending := range commit.DeletePending {
			if err := p.interrupts.Delete(ctx, pending.SessionID, pending.RootRunID); err != nil {
				return fmt.Errorf("runrecovery: delete Pending for root Run %q: %w", pending.RootRunID, err)
			}
		}
		if err := p.executorCheckpoints.DeleteUnownedCheckpoints(ctx, commit.PreservedCheckpointRootIDs); err != nil {
			return fmt.Errorf("runrecovery: delete unowned executor checkpoints: %w", err)
		}
		return nil
	})
}

var _ runs.RecoveryStore = (*Persistence)(nil)
