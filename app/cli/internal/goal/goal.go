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
func (s Status) AllowsLifecycleCommands() bool { return s != Completing }

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

func (r Reason) Validate() error {
	if !slices.Contains([]ReasonCode{
		StoppedByUser, RuntimeRestarted, RunStartFailed, AwaitingInput,
		TerminalOutcomeMissing, RunNotCompleted, RunBudgetReached,
		CostBudgetReached, StepBudgetReached, BlockedByModel,
	}, r.Code) {
		return fmt.Errorf("goal reason %q is invalid", r.Code)
	}
	return nil
}

type Budget struct {
	MaxRuns    int
	MaxCostUSD float64
	MaxSteps   int
}

func (b Budget) Validate() error {
	if b.MaxRuns < 0 || b.MaxCostUSD < 0 || b.MaxSteps < 0 ||
		math.IsNaN(b.MaxCostUSD) || math.IsInf(b.MaxCostUSD, 0) {
		return errors.New("goal budget contains a non-finite or negative limit")
	}
	return nil
}

type Usage struct {
	Runs    int
	CostUSD float64
	Steps   int
}

func (u Usage) Validate() error {
	if u.Runs < 0 || u.CostUSD < 0 || u.Steps < 0 ||
		math.IsNaN(u.CostUSD) || math.IsInf(u.CostUSD, 0) {
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

func (g Goal) Validate() error {
	var problems []error
	if strings.TrimSpace(g.SessionID) == "" {
		problems = append(problems, errors.New("session id is empty"))
	}
	if strings.TrimSpace(g.Objective) == "" {
		problems = append(problems, errors.New("objective is empty"))
	}
	if g.Status != Active && g.Status != Paused && g.Status != Blocked && g.Status != Completing {
		problems = append(problems, fmt.Errorf("status %q is invalid", g.Status))
	}
	if (g.Status == Active || g.Status == Completing) && g.Reason != nil {
		problems = append(problems, errors.New("non-resting goal carries a stopping reason"))
	}
	if (g.Status == Paused || g.Status == Blocked) && g.Reason == nil {
		problems = append(problems, errors.New("resting goal has no reason"))
	}
	if g.Reason != nil {
		problems = append(problems, g.Reason.Validate())
	}
	if (g.Provider == "") != (g.Model == "") {
		problems = append(problems, errors.New("provider and model must both be set or both be empty"))
	} else if g.Provider != strings.TrimSpace(g.Provider) || g.Model != strings.TrimSpace(g.Model) {
		problems = append(problems, errors.New("provider and model must not have surrounding whitespace"))
	}
	problems = append(problems, g.Budget.Validate(), g.Used.Validate())
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

func (u Update) Validate() error {
	if strings.TrimSpace(u.SessionID) == "" {
		return errors.New("update goal: session id is empty")
	}
	if strings.TrimSpace(u.Objective) == "" {
		return errors.New("update goal: objective is empty")
	}
	if u.SessionID != strings.TrimSpace(u.SessionID) || u.Objective != strings.TrimSpace(u.Objective) {
		return errors.New("update goal: values must not have surrounding whitespace")
	}
	return nil
}

func (u Update) ValidateResult(result Goal) error {
	if err := u.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.SessionID != u.SessionID {
		problems = append(problems, fmt.Errorf("runtime returned session %q, want %q", result.SessionID, u.SessionID))
	}
	if result.Objective != u.Objective {
		problems = append(problems, fmt.Errorf("runtime returned objective %q, want %q", result.Objective, u.Objective))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("update goal: %w", err)
	}
	return nil
}

func (s Start) Validate() error {
	if strings.TrimSpace(s.SessionID) == "" {
		return errors.New("start goal: session id is empty")
	}
	if strings.TrimSpace(s.Objective) == "" {
		return errors.New("start goal: objective is empty")
	}
	if (s.Provider == "") != (s.Model == "") {
		return errors.New("start goal: provider and model must both be set or both be empty")
	}
	if s.Provider != strings.TrimSpace(s.Provider) || s.Model != strings.TrimSpace(s.Model) {
		return errors.New("start goal: provider and model must not have surrounding whitespace")
	}
	if err := s.Budget.Validate(); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	return nil
}

// ValidateResult verifies that a successful start acknowledgement represents
// the fresh objective incarnation requested by the caller.
func (s Start) ValidateResult(result Goal) error {
	if err := s.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.SessionID != s.SessionID {
		problems = append(problems, fmt.Errorf("runtime returned session %q, want %q", result.SessionID, s.SessionID))
	}
	if result.Objective != s.Objective {
		problems = append(problems, fmt.Errorf("runtime returned objective %q, want %q", result.Objective, s.Objective))
	}
	if result.Status != Active {
		problems = append(problems, fmt.Errorf("runtime returned status %q, want %q", result.Status, Active))
	}
	if result.Provider != s.Provider || result.Model != s.Model {
		problems = append(problems, fmt.Errorf(
			"runtime returned model %q/%q, want %q/%q",
			result.Provider, result.Model, s.Provider, s.Model,
		))
	}
	if result.Budget != s.Budget {
		problems = append(problems, fmt.Errorf("runtime returned budget %+v, want %+v", result.Budget, s.Budget))
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
