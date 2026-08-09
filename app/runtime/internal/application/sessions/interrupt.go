package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

const turnCleanupTimeout = 5 * time.Second

// RunExecutionBinding identifies a parked executor that a Session mutation
// must tear down after its durable write-set commits.
type RunExecutionBinding struct {
	RunID            string
	SessionID        string
	ExecutorID       string
	CheckpointRootID string
}

func (r RunExecutionBinding) executorRef() runs.ExecutorRef {
	return runs.ExecutorRef{SessionID: r.SessionID, ExecutorID: r.ExecutorID}
}

// ListOpenInterrupts exposes the run-admission read needed by application/runs.
func (c *Coordinator) ListOpenInterrupts(ctx context.Context, sessionID string) ([]runs.Pending, error) {
	if c.interrupts == nil {
		return nil, errors.New("sessions: interrupt store is unavailable")
	}
	return c.interrupts.List(ctx, sessionID)
}

// ActiveRun returns the Session's non-terminal Run — the one the admission
// invariant allows at most one of — reporting false when the Session holds none.
//
// It reads the durable record rather than a live registry: a Run parked on a person,
// or admitted before a restart, is just as much "the Run this Session already has",
// and the registry knows neither.
func (c *Coordinator) ActiveRun(ctx context.Context, sessionID string) (transcript.Run, bool, error) {
	runs, err := c.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return transcript.Run{}, false, err
	}
	for _, run := range runs {
		if !run.State.IsTerminal() {
			return run, true, nil
		}
	}
	return transcript.Run{}, false, nil
}

// LookupOpenInterrupt returns the parked run identified by runID without claiming
// or consuming it. The run use case owns the subsequent admission ordering.
func (c *Coordinator) LookupOpenInterrupt(ctx context.Context, runID string) (runs.Pending, bool, error) {
	if c.interrupts == nil {
		return runs.Pending{}, false, errors.New("sessions: interrupt store is unavailable")
	}
	return c.interrupts.Get(ctx, runID)
}

// ApplyRunCancel commits the atomic durable abandon write-set. Executor
// teardown is owned by application/runs for run commands; session rollback and
// deletion continue to use the coordinator's narrow cleanup collaborator.
func (c *Coordinator) ApplyRunCancel(ctx context.Context, sessionID, runID, reason string, finishedAt time.Time) (transcript.Run, error) {
	return c.terminalizeParkedRun(ctx, sessionID, runID, finishedAt, rundomain.OutcomeCanceled, reason)
}

// ApplyRunLost atomically ends a parked run whose executor checkpoint cannot be
// restored. It uses the recovery transition because the waiting Run never
// resumed into a normal executor terminal path.
func (c *Coordinator) ApplyRunLost(ctx context.Context, sessionID, runID string, finishedAt time.Time) error {
	_, err := c.terminalizeParkedRun(ctx, sessionID, runID, finishedAt, rundomain.OutcomeLost, "")
	return err
}

func (c *Coordinator) terminalizeParkedRun(ctx context.Context, sessionID, runID string, finishedAt time.Time, outcome rundomain.Outcome, detail string) (transcript.Run, error) {
	if finishedAt.IsZero() {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: finished time is required", runID)
	}
	if c.interrupts == nil || c.snapshots == nil || c.writes == nil {
		return transcript.Run{}, errors.New("sessions: interrupt lifecycle persistence is unavailable")
	}
	pending, found, err := c.interrupts.Get(ctx, runID)
	if err != nil {
		return transcript.Run{}, err
	}
	if !found || pending.SessionID != sessionID {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: open interrupt not found for session %q", runID, sessionID)
	}
	snapshot, err := c.snapshots.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return transcript.Run{}, err
	}
	plan, rootRun, err := (parkedRunTerminalization{
		sessionID: sessionID, rootRunID: runID, finishedAt: finishedAt,
		outcome: outcome, detail: detail, pending: pending, snapshot: snapshot,
	}).build()
	if err != nil {
		return transcript.Run{}, err
	}
	if err := c.writes.ApplyTerminal(ctx, plan); err != nil {
		return transcript.Run{}, err
	}
	// One write-set ended the run and dropped the set it was parked on, so one place
	// reports both — for a cancel and for a park declared unresumable alike. The run
	// command layer deliberately does not publish this again: it would be a second
	// author for the same commit, and only one of them would ever be updated.
	runIDs := make([]string, len(plan.Runs))
	for index, run := range plan.Runs {
		runIDs[index] = run.ID
	}
	notices := []change.Notice{
		change.InSession(change.Runs, sessionID, runIDs...),
		change.InSession(change.Interrupts, sessionID, runID),
		change.InSession(change.Sessions, sessionID),
	}
	if plan.GoalRun != nil {
		notices = append(notices, change.InSession(change.Goals, sessionID))
	}
	c.changed.Notify(notices...)
	return rootRun, nil
}

func (c *Coordinator) parkedExecutions(ctx context.Context, runIDs []string) ([]RunExecutionBinding, error) {
	var out []RunExecutionBinding
	for _, runID := range runIDs {
		if c.interrupts == nil {
			return nil, errors.New("sessions: interrupt store is unavailable")
		}
		pending, found, err := c.interrupts.Get(ctx, runID)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		root, ok := pending.RootContinuation()
		if !ok {
			return nil, fmt.Errorf("sessions: parked run %q has no root continuation", pending.RootRunID)
		}
		out = append(out, RunExecutionBinding{
			RunID:            pending.RootRunID,
			SessionID:        pending.SessionID,
			ExecutorID:       pending.ExecutorID,
			CheckpointRootID: root.MemberID,
		})
	}
	return out, nil
}

func (c *Coordinator) releaseExecution(ctx context.Context, r RunExecutionBinding) error {
	if c.executionReleaser == nil {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), turnCleanupTimeout)
	defer cancel()
	if err := c.executionReleaser.Release(cleanupCtx, r.executorRef()); err != nil {
		return fmt.Errorf("sessions: release executor %q for Run %q: %w", r.ExecutorID, r.RunID, err)
	}
	return nil
}
