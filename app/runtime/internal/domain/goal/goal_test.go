package goal

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

func TestNewValidates(t *testing.T) {
	now := time.Unix(0, 0)
	if _, err := New("", "obj", modelref.Selection{}, Budget{}, run.Capabilities{}, "lease", now); err == nil {
		t.Fatal("empty session should error")
	}
	if _, err := New("s", "", modelref.Selection{}, Budget{}, run.Capabilities{}, "lease", now); err == nil {
		t.Fatal("empty objective should error")
	}
	if _, err := New("s", "obj", modelref.Selection{}, Budget{MaxRuns: -1}, run.Capabilities{}, "lease", now); err == nil {
		t.Fatal("negative budget should error")
	}
	selection, err := modelref.New("p", "m")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New("s", "obj", selection, Budget{}, run.Capabilities{}, "", now); err == nil {
		t.Fatal("empty incarnation should error")
	}
	g, err := New("s", "obj", selection, Budget{MaxRuns: 3}, run.Capabilities{}, "lease", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if g.Status != StatusActive {
		t.Fatalf("new goal status = %q, want active", g.Status)
	}
}

func TestGoalOwnsFrozenRunCapabilities(t *testing.T) {
	input := run.Capabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Question, interrupt.Approval},
	}
	g, err := New("s", "obj", modelref.Selection{}, Budget{}, input, "lease", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := run.Capabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
	}
	validationErr := g.Capabilities.Validate()
	if !g.Capabilities.Equal(want) || validationErr != nil {
		t.Fatalf("capabilities = %+v err=%v, want canonical %+v", g.Capabilities, validationErr, want)
	}
	input.InterruptKinds[0] = interrupt.Approval
	if !g.Capabilities.Equal(want) {
		t.Fatalf("Goal shares caller capability storage: %+v", g.Capabilities)
	}
	clone := g.Clone()
	clone.Capabilities.InterruptKinds[0] = interrupt.Question
	if !g.Capabilities.Equal(want) {
		t.Fatalf("Goal.Clone shares capability storage: %+v", g.Capabilities)
	}
}

func TestGoalRejectsNonFiniteBudgetAndUsage(t *testing.T) {
	for name, value := range map[string]float64{
		"NaN":               math.NaN(),
		"positive infinity": math.Inf(1),
		"negative infinity": math.Inf(-1),
	} {
		t.Run("budget "+name, func(t *testing.T) {
			if _, err := New(
				"s", "obj", modelref.Selection{}, Budget{MaxCostUSD: value},
				run.Capabilities{},
				"lease", time.Unix(0, 0),
			); err == nil {
				t.Fatal("New accepted a non-finite goal budget")
			}
		})
		t.Run("usage "+name, func(t *testing.T) {
			goal, err := New("s", "obj", modelref.Selection{}, Budget{}, run.Capabilities{}, "lease", time.Unix(0, 0))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			goal.Revision = 1
			goal.Used.CostUSD = value
			if err := goal.ValidateSnapshot(); err == nil {
				t.Fatal("ValidateSnapshot accepted non-finite goal usage")
			}
		})
	}
}

func TestValidateSnapshotRejectsMissingConcurrencyIdentity(t *testing.T) {
	g, err := New("s", "obj", modelref.Selection{}, Budget{}, run.Capabilities{}, "lease", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Goal)
	}{
		{"incarnation", func(g *Goal) { g.IncarnationID = "" }},
		{"revision", func(g *Goal) { g.Revision = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corrupt := g
			tt.mutate(&corrupt)
			if err := corrupt.ValidateSnapshot(); err == nil {
				t.Fatalf("ValidateSnapshot accepted missing %s", tt.name)
			}
		})
	}
}

func TestResumeRejectsSpentBudget(t *testing.T) {
	now := time.Unix(0, 0)
	g, err := New("s", "obj", modelref.Selection{}, Budget{MaxRuns: 1}, run.Capabilities{}, "lease", now)
	if err != nil {
		t.Fatal(err)
	}
	g.AddRun(0, 0, now)
	g.Block(ReasonRunBudgetReached, "", now)
	if err := g.Resume(now); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("Resume error = %v, want ErrBudgetExhausted", err)
	}
	if g.Status != StatusBlocked {
		t.Fatalf("status after rejected Resume = %q, want blocked", g.Status)
	}
}

func TestReviseObjectiveOpensANewIncarnationWithoutResettingGoalFacts(t *testing.T) {
	createdAt := time.Unix(10, 0)
	updatedAt := createdAt.Add(time.Minute)
	g, err := New(
		"s", "first", modelref.Selection{}, Budget{MaxRuns: 4},
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
		"lease-first", createdAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	g.AddRun(0.25, 2, createdAt.Add(time.Second))
	g.Pause(ReasonStoppedByUser, "", createdAt.Add(2*time.Second))

	if err := g.ReviseObjective("second", "lease-second", updatedAt); err != nil {
		t.Fatalf("ReviseObjective: %v", err)
	}
	if g.Objective != "second" || g.IncarnationID != "lease-second" {
		t.Fatalf("revised identity = %q/%q", g.Objective, g.IncarnationID)
	}
	if g.Status != StatusPaused || g.Reason.Code != ReasonStoppedByUser {
		t.Fatalf("revised lifecycle = %q/%+v", g.Status, g.Reason)
	}
	if g.Budget.MaxRuns != 4 || g.Used != (Usage{Runs: 1, CostUSD: 0.25, Steps: 2}) {
		t.Fatalf("revised accounting = budget %+v used %+v", g.Budget, g.Used)
	}
	if !g.CreatedAt.Equal(createdAt) || !g.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("revised times = created %v updated %v", g.CreatedAt, g.UpdatedAt)
	}
}

func TestBudgetExceeded(t *testing.T) {
	tests := []struct {
		name   string
		budget Budget
		used   Usage
		limit  BudgetLimit
		want   bool
	}{
		{"unbounded", Budget{}, Usage{Runs: 100, CostUSD: 999, Steps: 999}, BudgetLimitNone, false},
		{"under", Budget{MaxRuns: 5}, Usage{Runs: 4}, BudgetLimitNone, false},
		{"Runs", Budget{MaxRuns: 5}, Usage{Runs: 5}, BudgetLimitRuns, true},
		{"cost", Budget{MaxCostUSD: 1.0}, Usage{CostUSD: 1.0}, BudgetLimitCost, true},
		{"steps", Budget{MaxSteps: 10}, Usage{Steps: 11}, BudgetLimitSteps, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, ok := tt.budget.Exceeded(tt.used)
			if ok != tt.want || limit != tt.limit {
				t.Fatalf("Exceeded = (%v, %v), want (%v, %v)", limit, ok, tt.limit, tt.want)
			}
		})
	}
}

func TestRecordRunPreservesPriorTerminalReport(t *testing.T) {
	now := time.Unix(0, 0)
	g, err := New("s", "obj", modelref.Selection{}, Budget{MaxRuns: 1}, run.Capabilities{}, "lease", now)
	if err != nil {
		t.Fatal(err)
	}
	g.Block(ReasonBlockedByModel, "need a credential", now)
	g.RecordRun(RunRecord{
		SessionID: "s", IncarnationID: "lease", RunID: "run_1", Outcome: run.OutcomeCompleted,
		CostUSD: 0.25, Steps: 2, CompletedAt: now.Add(time.Second),
	})
	if g.Status != StatusBlocked || g.Reason != (Reason{Code: ReasonBlockedByModel, Detail: "need a credential"}) {
		t.Fatalf("status/reason after terminal record = %q/%+v", g.Status, g.Reason)
	}
	if g.Used != (Usage{Runs: 1, CostUSD: 0.25, Steps: 2}) {
		t.Fatalf("usage after terminal record = %+v", g.Used)
	}
}

func TestTransitions(t *testing.T) {
	now := time.Unix(0, 0)
	g, _ := New("s", "obj", modelref.Selection{}, Budget{}, run.Capabilities{}, "lease", now)

	g.AddRun(0.5, 2, now)
	g.AddRun(0.25, 1, now)
	if g.Used.Runs != 2 || g.Used.CostUSD != 0.75 || g.Used.Steps != 3 {
		t.Fatalf("usage accumulation = %+v", g.Used)
	}

	g.Block(ReasonRunBudgetReached, "", now)
	if g.Status != StatusBlocked || g.Reason != (Reason{Code: ReasonRunBudgetReached}) {
		t.Fatalf("Block = (%q, %+v)", g.Status, g.Reason)
	}
	g.Resume(now)
	if g.Status != StatusActive || g.Reason != (Reason{}) {
		t.Fatalf("Resume = (%q, %+v), want (active, zero reason)", g.Status, g.Reason)
	}
	g.Pause(ReasonStoppedByUser, "", now)
	if g.Status != StatusPaused || g.Reason != (Reason{Code: ReasonStoppedByUser}) {
		t.Fatalf("Pause = (%q, %+v)", g.Status, g.Reason)
	}
}
