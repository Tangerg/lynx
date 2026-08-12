// Package goal defines autonomous-session objective lifecycle values and its
// consumer-owned runtime port.
package goal

import (
	"context"
	"errors"
	"fmt"
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
	if budget.MaxRuns < 0 || budget.MaxCostUSD < 0 || budget.MaxSteps < 0 {
		return errors.New("goal budget contains a negative limit")
	}
	return nil
}

type Usage struct {
	Runs    int
	CostUSD float64
	Steps   int
}

func (usage Usage) Validate() error {
	if usage.Runs < 0 || usage.CostUSD < 0 || usage.Steps < 0 {
		return errors.New("goal usage contains a negative value")
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
	if err := start.Budget.Validate(); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	return nil
}

type Service interface {
	GetGoal(context.Context, string) (Goal, bool, error)
	StartGoal(context.Context, Start) (Goal, error)
	StopGoal(context.Context, string) (Goal, error)
	ResumeGoal(context.Context, string) (Goal, error)
}
