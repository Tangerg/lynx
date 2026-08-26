// Package goal defines autonomous-session objective lifecycle values and its
// consumer-owned runtime port.
package goal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

type Status string

const (
	Active     Status = "active"
	Paused     Status = "paused"
	Blocked    Status = "blocked"
	Completing Status = "completing"
)

// AllowsLifecycleCommands reports whether a start, stop, or resume request can
// be meaningful in this observed state. The runtime remains authoritative for
// concurrent transitions between the observation and a command.
func (status Status) AllowsLifecycleCommands() bool { return status != Completing }

type ReasonCode string

const (
	StoppedByUser          ReasonCode = "stoppedByUser"
	RuntimeRestarted       ReasonCode = "runtimeRestarted"
	RunStartFailed         ReasonCode = "runStartFailed"
	AwaitingInput          ReasonCode = "awaitingInput"
	TerminalOutcomeMissing ReasonCode = "terminalOutcomeMissing"
	RunNotCompleted        ReasonCode = "runNotCompleted"
	RunBudgetReached       ReasonCode = "runBudgetReached"
	CostBudgetReached      ReasonCode = "costBudgetReached"
	StepBudgetReached      ReasonCode = "stepBudgetReached"
	BlockedByModel         ReasonCode = "blockedByModel"
)

type Reason struct {
	Code   ReasonCode
	Detail string
}

func (reason Reason) Validate() error {
	if !slices.Contains([]ReasonCode{
		StoppedByUser, RuntimeRestarted, RunStartFailed, AwaitingInput,
		TerminalOutcomeMissing, RunNotCompleted, RunBudgetReached,
		CostBudgetReached, StepBudgetReached, BlockedByModel,
	}, reason.Code) {
		return fmt.Errorf("goal reason %q is invalid", reason.Code)
	}
	return nil
}

type Budget struct {
	MaxRuns    int
	MaxCostUSD float64
	MaxSteps   int
}

func (budget Budget) Validate() error {
	if budget.MaxRuns < 0 || budget.MaxCostUSD < 0 || budget.MaxSteps < 0 ||
		math.IsNaN(budget.MaxCostUSD) || math.IsInf(budget.MaxCostUSD, 0) {
		return errors.New("goal budget contains a non-finite or negative limit")
	}
	return nil
}

type Usage struct {
	Runs    int
	CostUSD float64
	Steps   int
}

func (usage Usage) Validate() error {
	if usage.Runs < 0 || usage.CostUSD < 0 || usage.Steps < 0 ||
		math.IsNaN(usage.CostUSD) || math.IsInf(usage.CostUSD, 0) {
		return errors.New("goal usage contains a non-finite or negative value")
	}
	return nil
}

type Goal struct {
	SessionID string
	Objective string
	Status    Status
	Reason    *Reason
	Provider  string
	Model     string
	Budget    Budget
	Used      Usage
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (goal Goal) Validate() error {
	var problems []error
	if strings.TrimSpace(goal.SessionID) == "" {
		problems = append(problems, errors.New("session id is empty"))
	}
	if strings.TrimSpace(goal.Objective) == "" {
		problems = append(problems, errors.New("objective is empty"))
	}
	if goal.Status != Active && goal.Status != Paused && goal.Status != Blocked && goal.Status != Completing {
		problems = append(problems, fmt.Errorf("status %q is invalid", goal.Status))
	}
	if (goal.Status == Active || goal.Status == Completing) && goal.Reason != nil {
		problems = append(problems, errors.New("non-resting goal carries a stopping reason"))
	}
	if (goal.Status == Paused || goal.Status == Blocked) && goal.Reason == nil {
		problems = append(problems, errors.New("resting goal has no reason"))
	}
	if goal.Reason != nil {
		problems = append(problems, goal.Reason.Validate())
	}
	if (goal.Provider == "") != (goal.Model == "") {
		problems = append(problems, errors.New("provider and model must both be set or both be empty"))
	} else if goal.Provider != strings.TrimSpace(goal.Provider) || goal.Model != strings.TrimSpace(goal.Model) {
		problems = append(problems, errors.New("provider and model must not have surrounding whitespace"))
	}
	problems = append(problems, goal.Budget.Validate(), goal.Used.Validate())
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("goal: %w", err)
	}
	return nil
}

type Start struct {
	SessionID string
	Objective string
	Provider  string
	Model     string
	Budget    Budget
}

// Update revises the objective of the current Goal without replacing its
// lifecycle, model selection, budget, or accumulated usage.
type Update struct {
	SessionID string
	Objective string
}

func (update Update) Validate() error {
	if strings.TrimSpace(update.SessionID) == "" {
		return errors.New("update goal: session id is empty")
	}
	if strings.TrimSpace(update.Objective) == "" {
		return errors.New("update goal: objective is empty")
	}
	if update.SessionID != strings.TrimSpace(update.SessionID) || update.Objective != strings.TrimSpace(update.Objective) {
		return errors.New("update goal: values must not have surrounding whitespace")
	}
	return nil
}

func (update Update) ValidateResult(result Goal) error {
	if err := update.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.SessionID != update.SessionID {
		problems = append(problems, fmt.Errorf("runtime returned session %q, want %q", result.SessionID, update.SessionID))
	}
	if result.Objective != update.Objective {
		problems = append(problems, fmt.Errorf("runtime returned objective %q, want %q", result.Objective, update.Objective))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("update goal: %w", err)
	}
	return nil
}

func (start Start) Validate() error {
	if strings.TrimSpace(start.SessionID) == "" {
		return errors.New("start goal: session id is empty")
	}
	if strings.TrimSpace(start.Objective) == "" {
		return errors.New("start goal: objective is empty")
	}
	if (start.Provider == "") != (start.Model == "") {
		return errors.New("start goal: provider and model must both be set or both be empty")
	}
	if start.Provider != strings.TrimSpace(start.Provider) || start.Model != strings.TrimSpace(start.Model) {
		return errors.New("start goal: provider and model must not have surrounding whitespace")
	}
	if err := start.Budget.Validate(); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	return nil
}

// ValidateResult verifies that a successful start acknowledgement represents
// the fresh objective incarnation requested by the caller.
func (start Start) ValidateResult(result Goal) error {
	if err := start.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.SessionID != start.SessionID {
		problems = append(problems, fmt.Errorf("runtime returned session %q, want %q", result.SessionID, start.SessionID))
	}
	if result.Objective != start.Objective {
		problems = append(problems, fmt.Errorf("runtime returned objective %q, want %q", result.Objective, start.Objective))
	}
	if result.Status != Active {
		problems = append(problems, fmt.Errorf("runtime returned status %q, want %q", result.Status, Active))
	}
	if result.Provider != start.Provider || result.Model != start.Model {
		problems = append(problems, fmt.Errorf(
			"runtime returned model %q/%q, want %q/%q",
			result.Provider, result.Model, start.Provider, start.Model,
		))
	}
	if result.Budget != start.Budget {
		problems = append(problems, fmt.Errorf("runtime returned budget %+v, want %+v", result.Budget, start.Budget))
	}
	if result.Used != (Usage{}) {
		problems = append(problems, fmt.Errorf("runtime returned non-zero usage %+v for a new goal", result.Used))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	return nil
}

type Service interface {
	GetGoal(context.Context, string) (Goal, bool, error)
	StartGoal(context.Context, Start) (Goal, error)
	UpdateGoal(context.Context, Update) (Goal, error)
	ClearGoal(context.Context, string) error
	StopGoal(context.Context, string) (Goal, error)
	ResumeGoal(context.Context, string) (Goal, error)
}
