// Package goal owns the autonomous objective that spans multiple root Runs.
// It contains no scheduler, protocol DTO, database shape or agent mechanism.
package goal

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var (
	ErrInvalid           = errors.New("goal: invalid state")
	ErrInvalidTransition = errors.New("goal: invalid transition")
	ErrVersionConflict   = errors.New("goal: version conflict")
	ErrNotFound          = errors.New("goal: not found")
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
	ReasonNone                   ReasonCode = ""
	ReasonStoppedByUser          ReasonCode = "stoppedByUser"
	ReasonRuntimeRestarted       ReasonCode = "runtimeRestarted"
	ReasonRunStartFailed         ReasonCode = "runStartFailed"
	ReasonAwaitingInput          ReasonCode = "awaitingInput"
	ReasonTerminalOutcomeMissing ReasonCode = "terminalOutcomeMissing"
	ReasonRunNotCompleted        ReasonCode = "runNotCompleted"
	ReasonRunBudgetReached       ReasonCode = "runBudgetReached"
	ReasonCostBudgetReached      ReasonCode = "costBudgetReached"
	ReasonStepBudgetReached      ReasonCode = "stepBudgetReached"
	ReasonBlockedByModel         ReasonCode = "blockedByModel"
)

type Reason struct {
	Code   ReasonCode
	Detail string
}

type Budget struct {
	MaxRuns    int
	MaxCostUSD float64
	MaxSteps   int
}

func (value Budget) Validate() error {
	if value.MaxRuns < 0 || value.MaxSteps < 0 || value.MaxCostUSD < 0 || math.IsNaN(value.MaxCostUSD) || math.IsInf(value.MaxCostUSD, 0) {
		return fmt.Errorf("%w: budget must be finite and non-negative", ErrInvalid)
	}
	return nil
}

type Usage struct {
	Runs    int
	CostUSD float64
	Steps   int
}

type Goal struct {
	sessionID, incarnationID, objective string
	provider, model, activeRunID        string
	status                              Status
	reason                              Reason
	budget                              Budget
	used                                Usage
	revision                            uint64
	createdAt, updatedAt                time.Time
}

type Create struct {
	SessionID, IncarnationID, Objective string
	Provider, Model                     string
	Budget                              Budget
	Now                                 time.Time
}

func New(command Create) (Goal, error) {
	value := Goal{
		sessionID: strings.TrimSpace(command.SessionID), incarnationID: strings.TrimSpace(command.IncarnationID),
		objective: strings.TrimSpace(command.Objective), provider: strings.TrimSpace(command.Provider), model: strings.TrimSpace(command.Model),
		status: Active, budget: command.Budget, revision: 1,
		createdAt: command.Now.UTC(), updatedAt: command.Now.UTC(),
	}
	if err := value.Validate(); err != nil {
		return Goal{}, err
	}
	return value, nil
}

type Restore struct {
	SessionID, IncarnationID, Objective string
	Provider, Model, ActiveRunID        string
	Status                              Status
	Reason                              Reason
	Budget                              Budget
	Used                                Usage
	Revision                            uint64
	CreatedAt, UpdatedAt                time.Time
}

func Rehydrate(snapshot Restore) (Goal, error) {
	value := Goal{
		sessionID: snapshot.SessionID, incarnationID: snapshot.IncarnationID, objective: snapshot.Objective,
		provider: snapshot.Provider, model: snapshot.Model, activeRunID: snapshot.ActiveRunID,
		status: snapshot.Status, reason: snapshot.Reason, budget: snapshot.Budget, used: snapshot.Used,
		revision: snapshot.Revision, createdAt: snapshot.CreatedAt.UTC(), updatedAt: snapshot.UpdatedAt.UTC(),
	}
	if err := value.Validate(); err != nil {
		return Goal{}, err
	}
	return value, nil
}

func (value Goal) Validate() error {
	if value.sessionID == "" || value.incarnationID == "" || value.objective == "" || value.revision == 0 || value.createdAt.IsZero() || value.updatedAt.IsZero() {
		return fmt.Errorf("%w: identity, objective, revision and timestamps are required", ErrInvalid)
	}
	if (value.provider == "") != (value.model == "") {
		return fmt.Errorf("%w: provider and model must be selected together", ErrInvalid)
	}
	if err := value.budget.Validate(); err != nil {
		return err
	}
	if value.used.Runs < 0 || value.used.Steps < 0 || value.used.CostUSD < 0 || math.IsNaN(value.used.CostUSD) || math.IsInf(value.used.CostUSD, 0) {
		return fmt.Errorf("%w: usage must be finite and non-negative", ErrInvalid)
	}
	switch value.status {
	case Active, Completing:
		if value.reason.Code != ReasonNone || value.reason.Detail != "" {
			return fmt.Errorf("%w: active lifecycle carries a stop reason", ErrInvalid)
		}
	case Paused, Blocked:
		if !value.reason.Code.valid() {
			return fmt.Errorf("%w: stopped lifecycle requires a known reason", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: unknown status %q", ErrInvalid, value.status)
	}
	return nil
}

func (code ReasonCode) valid() bool {
	switch code {
	case ReasonStoppedByUser, ReasonRuntimeRestarted, ReasonRunStartFailed, ReasonAwaitingInput,
		ReasonTerminalOutcomeMissing, ReasonRunNotCompleted, ReasonRunBudgetReached,
		ReasonCostBudgetReached, ReasonStepBudgetReached, ReasonBlockedByModel:
		return true
	default:
		return false
	}
}

func (value *Goal) ClaimRun(runID string, now time.Time) error {
	if value.status != Active || value.activeRunID != "" || strings.TrimSpace(runID) == "" {
		return ErrInvalidTransition
	}
	value.activeRunID = runID
	value.updatedAt = now.UTC()
	return value.Validate()
}

func (value *Goal) AwaitInput(runID string, now time.Time) error {
	if value.status != Active || value.activeRunID != runID {
		return ErrInvalidTransition
	}
	value.status = Blocked
	value.reason = Reason{Code: ReasonAwaitingInput}
	value.updatedAt = now.UTC()
	return value.Validate()
}

// ContinueRun reactivates the same owned Run after its user input interrupt was
// answered. It never admits a replacement Run or changes the incarnation.
func (value *Goal) ContinueRun(runID string, now time.Time) error {
	if value.status != Blocked || value.reason.Code != ReasonAwaitingInput || value.activeRunID != runID {
		return ErrInvalidTransition
	}
	value.status = Active
	value.reason = Reason{}
	value.updatedAt = now.UTC()
	return value.Validate()
}

func (value *Goal) Pause(code ReasonCode, detail string, now time.Time) error {
	if value.status != Active && (value.status != Blocked || value.reason.Code != ReasonAwaitingInput) {
		return ErrInvalidTransition
	}
	if code == ReasonNone || !code.valid() {
		return ErrInvalidTransition
	}
	value.status = Paused
	value.reason = Reason{Code: code, Detail: strings.TrimSpace(detail)}
	value.updatedAt = now.UTC()
	return value.Validate()
}

func (value *Goal) Resume(now time.Time) error {
	if (value.status != Paused && value.status != Blocked) || value.activeRunID != "" {
		return ErrInvalidTransition
	}
	if _, exceeded := value.exceeded(); exceeded {
		return ErrInvalidTransition
	}
	value.status = Active
	value.reason = Reason{}
	value.updatedAt = now.UTC()
	return value.Validate()
}

func (value *Goal) ReplaceObjective(objective, incarnationID string, now time.Time) error {
	if value.status == Completing || strings.TrimSpace(objective) == "" || strings.TrimSpace(incarnationID) == "" {
		return ErrInvalidTransition
	}
	value.objective = strings.TrimSpace(objective)
	value.incarnationID = strings.TrimSpace(incarnationID)
	value.activeRunID = ""
	value.updatedAt = now.UTC()
	return value.Validate()
}

func (value *Goal) Report(incarnationID string, completed bool, reason string, now time.Time) error {
	if value.status != Active || value.incarnationID != incarnationID {
		return ErrInvalidTransition
	}
	if completed {
		if strings.TrimSpace(reason) != "" {
			return ErrInvalidTransition
		}
		value.status = Completing
		value.reason = Reason{}
	} else {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return ErrInvalidTransition
		}
		value.status = Blocked
		value.reason = Reason{Code: ReasonBlockedByModel, Detail: reason}
	}
	value.updatedAt = now.UTC()
	return value.Validate()
}

type RunSettlement struct {
	RunID   string
	Outcome string
	CostUSD float64
	Steps   int
	Now     time.Time
}

// SettleRun folds one exact terminal Run. The bool result requests deletion
// only when a completed Run closes a model-reported completion window.
func (value *Goal) SettleRun(settlement RunSettlement) (bool, error) {
	if value.activeRunID != settlement.RunID || settlement.Steps < 0 || settlement.CostUSD < 0 || math.IsNaN(settlement.CostUSD) || math.IsInf(settlement.CostUSD, 0) {
		return false, ErrInvalidTransition
	}
	value.activeRunID = ""
	value.used.Runs++
	value.used.Steps += settlement.Steps
	value.used.CostUSD += settlement.CostUSD
	value.updatedAt = settlement.Now.UTC()
	// A user stop owns the lifecycle even if canceling its in-flight Run races
	// with terminal accounting. Settlement still charges the Run, but must not
	// rewrite the explicit pause as a model/runtime failure.
	if value.status == Paused {
		return false, value.Validate()
	}
	if settlement.Outcome != "completed" {
		value.status = Blocked
		if code, exceeded := value.exceeded(); exceeded {
			value.reason = Reason{Code: code}
		} else {
			value.reason = Reason{Code: ReasonRunNotCompleted, Detail: settlement.Outcome}
		}
		return false, value.Validate()
	}
	if value.status == Completing {
		return true, value.Validate()
	}
	if value.status == Blocked && value.reason.Code == ReasonBlockedByModel {
		return false, value.Validate()
	}
	if code, exceeded := value.exceeded(); exceeded {
		value.status = Blocked
		value.reason = Reason{Code: code}
	}
	return false, value.Validate()
}

// RecoverWithoutRun repairs an active lifecycle whose owned Run disappeared
// across a Runtime generation. Active work becomes an explicit restart pause;
// a completion window without its final Run becomes a blocked inconsistency.
func (value *Goal) RecoverWithoutRun(now time.Time) error {
	switch value.status {
	case Active:
		value.status = Paused
		value.reason = Reason{Code: ReasonRuntimeRestarted}
	case Completing:
		value.status = Blocked
		value.reason = Reason{Code: ReasonTerminalOutcomeMissing}
	default:
		return ErrInvalidTransition
	}
	value.activeRunID = ""
	value.updatedAt = now.UTC()
	return value.Validate()
}

func (value Goal) exceeded() (ReasonCode, bool) {
	switch {
	case value.budget.MaxRuns > 0 && value.used.Runs >= value.budget.MaxRuns:
		return ReasonRunBudgetReached, true
	case value.budget.MaxCostUSD > 0 && value.used.CostUSD >= value.budget.MaxCostUSD:
		return ReasonCostBudgetReached, true
	case value.budget.MaxSteps > 0 && value.used.Steps >= value.budget.MaxSteps:
		return ReasonStepBudgetReached, true
	default:
		return ReasonNone, false
	}
}

func (value Goal) WithRevision(revision uint64) (Goal, error) {
	value.revision = revision
	return value, value.Validate()
}

func (value Goal) SessionID() string     { return value.sessionID }
func (value Goal) IncarnationID() string { return value.incarnationID }
func (value Goal) Objective() string     { return value.objective }
func (value Goal) Provider() string      { return value.provider }
func (value Goal) Model() string         { return value.model }
func (value Goal) ActiveRunID() string   { return value.activeRunID }
func (value Goal) Status() Status        { return value.status }
func (value Goal) Reason() Reason        { return value.reason }
func (value Goal) Budget() Budget        { return value.budget }
func (value Goal) Used() Usage           { return value.used }
func (value Goal) Revision() uint64      { return value.revision }
func (value Goal) CreatedAt() time.Time  { return value.createdAt }
func (value Goal) UpdatedAt() time.Time  { return value.updatedAt }
