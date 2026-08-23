package goalflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	goaldomain "github.com/Tangerg/lynx/app2/runtime/domain/goal"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

type AutonomousRuns interface {
	WaitSessionStartable(context.Context, string) error
	StartAutonomous(context.Context, runflow.AutonomousStart) (*runflow.AutonomousRun, error)
	Get(context.Context, string) (*protocol.RunRef, error)
	Cancel(context.Context, protocol.CancelRunRequest) (*protocol.CancelRunResponse, error)
}

type DriverConfig struct {
	Goals    *Service
	Runs     AutonomousRuns
	Signals  *Signals
	Lifetime context.Context
}

type Driver struct {
	goals   *Service
	runs    AutonomousRuns
	signals *Signals
	life    context.Context
	cancel  context.CancelFunc

	mu         sync.Mutex
	drives     map[string]*drive
	suppressed map[string]bool
	tasks      sync.WaitGroup
	started    bool
	closing    bool
}

type drive struct {
	incarnationID string
	runID         string
	cancel        context.CancelFunc
}

func NewDriver(config DriverConfig) (*Driver, error) {
	if config.Goals == nil || config.Runs == nil || config.Signals == nil || config.Lifetime == nil {
		return nil, errors.New("goalflow: goals, runs, signals and lifetime are required")
	}
	life, cancel := context.WithCancel(config.Lifetime)
	return &Driver{goals: config.Goals, runs: config.Runs, signals: config.Signals, life: life, cancel: cancel, drives: make(map[string]*drive), suppressed: make(map[string]bool)}, nil
}

// Recover reconciles durable ownership after runflow has marked predecessor
// executions lost. It never silently resumes a Goal from an earlier Runtime
// generation: remaining active work becomes an explicit restart pause.
func (driver *Driver) Recover(ctx context.Context) error {
	values, err := driver.goals.store.ListGoals(ctx)
	if err != nil {
		return fmt.Errorf("goalflow: list goals for recovery: %w", err)
	}
	for _, value := range values {
		if err := driver.recoverOne(ctx, value); err != nil {
			if errors.Is(err, goaldomain.ErrVersionConflict) {
				continue
			}
			return fmt.Errorf("goalflow: recover session %s: %w", value.SessionID(), err)
		}
	}
	return nil
}

func (driver *Driver) recoverOne(ctx context.Context, value goaldomain.Goal) error {
	if value.ActiveRunID() == "" {
		if value.Status() == goaldomain.Active || value.Status() == goaldomain.Completing {
			_, err := driver.goals.recoverWithoutRun(ctx, value)
			return err
		}
		return nil
	}
	current, err := driver.runs.Get(ctx, value.ActiveRunID())
	if errors.Is(err, protocol.ErrRunNotFound) {
		_, err = driver.goals.recoverWithoutRun(ctx, value)
		return err
	}
	if err != nil {
		return err
	}
	switch current.Status {
	case protocol.RunStatusWaiting:
		if value.Status() == goaldomain.Active {
			_, err = driver.goals.awaitInput(ctx, value)
		}
		return err
	case protocol.RunStatusFinished:
		if current.Outcome == nil {
			return errors.New("finished Run has no outcome")
		}
		removed, settleErr := driver.goals.settleRun(ctx, value, current.ID, string(current.Outcome.Type), current.Metrics)
		if settleErr != nil || removed {
			return settleErr
		}
		refreshed, found, loadErr := driver.goals.Current(ctx, value.SessionID())
		if loadErr != nil || !found {
			return loadErr
		}
		if refreshed.ActiveRunID() == "" && (refreshed.Status() == goaldomain.Active || refreshed.Status() == goaldomain.Completing) {
			_, loadErr = driver.goals.recoverWithoutRun(ctx, refreshed)
		}
		return loadErr
	default:
		return fmt.Errorf("predecessor Run %s remained running after Run recovery", current.ID)
	}
}

func (driver *Driver) Start() {
	driver.mu.Lock()
	if driver.started || driver.closing {
		driver.mu.Unlock()
		return
	}
	driver.started = true
	driver.tasks.Add(1)
	driver.mu.Unlock()
	go driver.loop()
}

func (driver *Driver) loop() {
	defer driver.tasks.Done()
	for {
		select {
		case <-driver.signals.Wake():
			for _, sessionID := range driver.signals.Drain() {
				driver.reconcile(sessionID)
			}
		case <-driver.life.Done():
			return
		}
	}
}

func (driver *Driver) reconcile(sessionID string) {
	value, found, err := driver.goals.Current(driver.life, sessionID)
	if err != nil {
		driver.retry(sessionID)
		return
	}
	watchOwnedRun := false
	if found && value.ActiveRunID() != "" && (value.Status() == goaldomain.Completing ||
		(value.Status() == goaldomain.Blocked && value.Reason().Code == goaldomain.ReasonBlockedByModel)) {
		current, runErr := driver.runs.Get(driver.life, value.ActiveRunID())
		if runErr != nil {
			driver.retry(sessionID)
			return
		}
		watchOwnedRun = current.Status != protocol.RunStatusWaiting
	}
	driver.mu.Lock()
	existing := driver.drives[sessionID]
	if driver.suppressed[sessionID] {
		var cancel context.CancelFunc
		if existing != nil {
			cancel = existing.cancel
			delete(driver.drives, sessionID)
		}
		driver.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return
	}
	keep := found && existing != nil && existing.incarnationID == value.IncarnationID() &&
		(value.Status() == goaldomain.Active || value.Status() == goaldomain.Completing || value.Status() == goaldomain.Blocked)
	if keep {
		driver.mu.Unlock()
		return
	}
	var cancel context.CancelFunc
	var oldRunID string
	settlePaused := false
	if existing != nil {
		cancel, oldRunID = existing.cancel, existing.runID
		if oldRunID == "" && found && existing.incarnationID == value.IncarnationID() {
			oldRunID = value.ActiveRunID()
		}
		delete(driver.drives, sessionID)
		settlePaused = found && value.Status() == goaldomain.Paused && value.ActiveRunID() == oldRunID
	}
	if existing == nil && found && value.Status() == goaldomain.Paused && value.ActiveRunID() != "" {
		oldRunID, settlePaused = value.ActiveRunID(), true
	}
	shouldStart := found && (value.Status() == goaldomain.Active || watchOwnedRun)
	driver.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if oldRunID != "" {
		driver.cancelRun(oldRunID, "Goal lifecycle changed")
		if settlePaused {
			driver.settlePausedRun(sessionID, value.IncarnationID(), oldRunID)
		}
	}
	if shouldStart {
		driver.startDrive(value)
	}
}

func (driver *Driver) startDrive(value goaldomain.Goal) {
	ctx, cancel := context.WithCancel(driver.life)
	state := &drive{incarnationID: value.IncarnationID(), runID: value.ActiveRunID(), cancel: cancel}
	driver.mu.Lock()
	if driver.closing || driver.suppressed[value.SessionID()] || driver.drives[value.SessionID()] != nil {
		driver.mu.Unlock()
		cancel()
		return
	}
	driver.drives[value.SessionID()] = state
	driver.tasks.Add(1)
	driver.mu.Unlock()
	go func() {
		defer driver.tasks.Done()
		defer cancel()
		defer func() {
			driver.mu.Lock()
			if driver.drives[value.SessionID()] == state {
				delete(driver.drives, value.SessionID())
			}
			driver.mu.Unlock()
			driver.signals.Publish(value.SessionID())
		}()
		driver.driveGoal(ctx, state, value.SessionID(), value.IncarnationID())
	}()
}

func (driver *Driver) retry(sessionID string) {
	time.AfterFunc(250*time.Millisecond, func() {
		select {
		case <-driver.life.Done():
		default:
			driver.signals.Publish(sessionID)
		}
	})
}

func (driver *Driver) driveGoal(ctx context.Context, state *drive, sessionID, incarnationID string) {
	for ctx.Err() == nil {
		value, found, err := driver.goals.Current(ctx, sessionID)
		if err != nil || !found || value.IncarnationID() != incarnationID {
			return
		}
		if value.ActiveRunID() == "" {
			if value.Status() != goaldomain.Active {
				return
			}
			if err := driver.runs.WaitSessionStartable(ctx, sessionID); err != nil {
				return
			}
			opened, err := driver.runs.StartAutonomous(ctx, runflow.AutonomousStart{
				SessionID: sessionID, Instruction: goalInstruction(value), Provider: value.Provider(), Model: value.Model(),
				MaxSteps: remainingSteps(value), MaxBudgetUSD: remainingCost(value),
				Claim: func(claimCtx context.Context, runID string) error {
					if !driver.prepareClaim(sessionID, state, runID) {
						return context.Canceled
					}
					_, err := driver.goals.claimRun(claimCtx, sessionID, incarnationID, runID)
					return err
				},
			})
			if err != nil {
				if errors.Is(err, protocol.ErrSessionHasActiveRun) {
					continue
				}
				if ctx.Err() != nil || errors.Is(err, goaldomain.ErrVersionConflict) || errors.Is(err, goaldomain.ErrInvalidTransition) || errors.Is(err, goaldomain.ErrNotFound) {
					return
				}
				driver.pauseStartFailure(ctx, sessionID, incarnationID, err)
				return
			}
			driver.setRun(state, opened.RunID)
			for range opened.Events {
			}
		} else {
			driver.setRun(state, value.ActiveRunID())
			if err := driver.waitRunSettlement(ctx, value.ActiveRunID()); err != nil {
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		if !driver.settleCurrent(ctx, sessionID, incarnationID) {
			return
		}
	}
}

func (driver *Driver) prepareClaim(sessionID string, state *drive, runID string) bool {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if driver.closing || driver.suppressed[sessionID] || driver.drives[sessionID] != state {
		return false
	}
	state.runID = runID
	return true
}

func (driver *Driver) waitRunSettlement(ctx context.Context, runID string) error {
	const retry = 125 * time.Millisecond
	for {
		current, err := driver.runs.Get(ctx, runID)
		if err != nil {
			return err
		}
		if current.Status != protocol.RunStatusRunning {
			return nil
		}
		timer := time.NewTimer(retry)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		}
	}
}

func (driver *Driver) settleCurrent(ctx context.Context, sessionID, incarnationID string) bool {
	value, found, err := driver.goals.Current(ctx, sessionID)
	if err != nil || !found || value.IncarnationID() != incarnationID || value.ActiveRunID() == "" {
		return false
	}
	current, err := driver.runs.Get(ctx, value.ActiveRunID())
	if err != nil {
		return false
	}
	switch current.Status {
	case protocol.RunStatusWaiting:
		if value.Status() == goaldomain.Active {
			_, _ = driver.goals.awaitInput(ctx, value)
		}
		return false
	case protocol.RunStatusFinished:
		if current.Outcome == nil {
			return false
		}
		removed, err := driver.goals.settleRun(ctx, value, current.ID, string(current.Outcome.Type), current.Metrics)
		if err != nil || removed {
			return false
		}
		updated, found, err := driver.goals.Current(ctx, sessionID)
		return err == nil && found && updated.IncarnationID() == incarnationID && updated.Status() == goaldomain.Active
	default:
		return false
	}
}

// ObserveResumed reconnects a Goal to the exact owned Run after a user answers
// its interrupt. Run resume remains the source of execution truth; this only
// changes the Goal's blocked lifecycle and wakes its watcher.
func (driver *Driver) ObserveResumed(ctx context.Context, runID string) error {
	value, err := driver.goals.store.LoadGoalByRun(ctx, runID)
	if errors.Is(err, goaldomain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	driver.mu.Lock()
	suppressed := driver.closing || driver.suppressed[value.SessionID()]
	driver.mu.Unlock()
	if suppressed {
		return errors.New("goalflow: session lifecycle is changing")
	}
	if value.Status() == goaldomain.Blocked && value.Reason().Code == goaldomain.ReasonAwaitingInput {
		_, err = driver.goals.continueRun(ctx, runID)
		return err
	}
	if value.Status() == goaldomain.Completing || (value.Status() == goaldomain.Blocked && value.Reason().Code == goaldomain.ReasonBlockedByModel) {
		driver.signals.Publish(value.SessionID())
	}
	return nil
}

func (driver *Driver) CancelDetached(runID string) {
	if runID != "" {
		driver.cancelRun(runID, "Goal cleared")
	}
}

// SuppressSession closes Goal admission around a destructive Session
// transaction. The database transaction remains the resource owner; this
// method only quiesces process effects and returns whether a Goal existed.
func (driver *Driver) SuppressSession(ctx context.Context, sessionID string) (bool, error) {
	driver.mu.Lock()
	driver.suppressed[sessionID] = true
	state := driver.drives[sessionID]
	stateRunID := ""
	if state != nil {
		state.cancel()
		stateRunID = state.runID
		delete(driver.drives, sessionID)
	}
	driver.mu.Unlock()
	value, found, err := driver.goals.Current(ctx, sessionID)
	if err != nil {
		driver.ReleaseSession(sessionID, false)
		return false, err
	}
	runID := stateRunID
	if found && value.ActiveRunID() != "" {
		runID = value.ActiveRunID()
	}
	if runID != "" {
		driver.cancelRun(runID, "Session lifecycle changed")
	}
	return found, nil
}

// ReleaseSession reopens admission after the surrounding Session transaction.
// changed means that transaction deleted/replaced the Goal row and therefore
// owes the canonical invalidation through the Goal owner.
func (driver *Driver) ReleaseSession(sessionID string, changed bool) {
	driver.mu.Lock()
	delete(driver.suppressed, sessionID)
	closing := driver.closing
	driver.mu.Unlock()
	if closing {
		return
	}
	if changed {
		driver.goals.changed(sessionID)
		return
	}
	driver.signals.Publish(sessionID)
}

func (driver *Driver) setRun(state *drive, runID string) {
	driver.mu.Lock()
	state.runID = runID
	driver.mu.Unlock()
}

func (driver *Driver) pauseStartFailure(ctx context.Context, sessionID, incarnationID string, cause error) {
	value, found, err := driver.goals.Current(ctx, sessionID)
	if err != nil || !found || value.IncarnationID() != incarnationID || value.Status() != goaldomain.Active || value.ActiveRunID() != "" {
		return
	}
	_, _ = driver.goals.pause(ctx, value, goaldomain.ReasonRunStartFailed, cause.Error())
}

func (driver *Driver) cancelRun(runID, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := driver.runs.Cancel(ctx, protocol.CancelRunRequest{RunID: runID, Reason: reason})
	if errors.Is(err, protocol.ErrRunFinished) || errors.Is(err, protocol.ErrRunNotFound) {
		return
	}
}

func (driver *Driver) settlePausedRun(sessionID, incarnationID, runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	value, found, err := driver.goals.Current(ctx, sessionID)
	if err != nil || !found || value.IncarnationID() != incarnationID || value.Status() != goaldomain.Paused || value.ActiveRunID() != runID {
		return
	}
	current, err := driver.runs.Get(ctx, runID)
	if err != nil || current.Status != protocol.RunStatusFinished || current.Outcome == nil {
		return
	}
	_, _ = driver.goals.settleRun(ctx, value, runID, string(current.Outcome.Type), current.Metrics)
}

func (driver *Driver) Close() {
	driver.mu.Lock()
	if driver.closing {
		driver.mu.Unlock()
		return
	}
	driver.closing = true
	driver.cancel()
	runIDs := make([]string, 0, len(driver.drives))
	for _, value := range driver.drives {
		value.cancel()
		if value.runID != "" {
			runIDs = append(runIDs, value.runID)
		}
	}
	driver.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	goals, _ := driver.goals.store.ListGoals(ctx)
	for _, goal := range goals {
		if goal.Status() == goaldomain.Active {
			_, _ = driver.goals.pause(ctx, goal, goaldomain.ReasonRuntimeRestarted, "")
		}
		if goal.ActiveRunID() != "" {
			runIDs = append(runIDs, goal.ActiveRunID())
		}
	}
	seen := make(map[string]bool, len(runIDs))
	for _, runID := range runIDs {
		if !seen[runID] {
			seen[runID] = true
			driver.cancelRun(runID, "Runtime shutting down")
		}
	}
	for _, goal := range goals {
		if goal.ActiveRunID() == "" {
			continue
		}
		driver.settlePausedRun(goal.SessionID(), goal.IncarnationID(), goal.ActiveRunID())
	}
	driver.tasks.Wait()
}

func remainingSteps(value goaldomain.Goal) int {
	budget, used := value.Budget(), value.Used()
	if budget.MaxSteps == 0 {
		return 0
	}
	return max(0, budget.MaxSteps-used.Steps)
}

func remainingCost(value goaldomain.Goal) float64 {
	budget, used := value.Budget(), value.Used()
	if budget.MaxCostUSD == 0 {
		return 0
	}
	return max(0, budget.MaxCostUSD-used.CostUSD)
}

func goalInstruction(value goaldomain.Goal) string {
	var builder strings.Builder
	builder.WriteString("Autonomous Lyra Goal:\n")
	builder.WriteString(value.Objective())
	builder.WriteString("\n\nTake the next concrete step toward the full objective. Inspect existing work before changing it. Continue across Runs when more work remains. Call report_goal_outcome with completed only when the entire objective is genuinely achieved, or blocked with a concrete reason only when autonomous progress is impossible. Do not report an outcome merely because this Run is ending.")
	return builder.String()
}
