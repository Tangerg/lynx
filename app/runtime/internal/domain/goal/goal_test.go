package goal

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func TestNewValidates(t *testing.T) {
	now := time.Unix(0, 0)
	if _, err := New("", "obj", modelref.Selection{}, Budget{}, now); err == nil {
		t.Fatal("empty session should error")
	}
	if _, err := New("s", "", modelref.Selection{}, Budget{}, now); err == nil {
		t.Fatal("empty objective should error")
	}
	if _, err := New("s", "obj", modelref.Selection{}, Budget{MaxTurns: -1}, now); err == nil {
		t.Fatal("negative budget should error")
	}
	selection, err := modelref.New("p", "m")
	if err != nil {
		t.Fatal(err)
	}
	g, err := New("s", "obj", selection, Budget{MaxTurns: 3}, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if g.Status != StatusActive {
		t.Fatalf("new goal status = %q, want active", g.Status)
	}
}

func TestResumeRejectsSpentBudget(t *testing.T) {
	now := time.Unix(0, 0)
	g, err := New("s", "obj", modelref.Selection{}, Budget{MaxTurns: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	g.AddTurn(0, 0, now)
	g.Block(ReasonTurnBudgetReached, "", now)
	if err := g.Resume(now); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("Resume error = %v, want ErrBudgetExhausted", err)
	}
	if g.Status != StatusBlocked {
		t.Fatalf("status after rejected Resume = %q, want blocked", g.Status)
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
		{"unbounded", Budget{}, Usage{Turns: 100, CostUSD: 999, Steps: 999}, BudgetLimitNone, false},
		{"under", Budget{MaxTurns: 5}, Usage{Turns: 4}, BudgetLimitNone, false},
		{"turns", Budget{MaxTurns: 5}, Usage{Turns: 5}, BudgetLimitTurns, true},
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

func TestRecordTurnPreservesPriorTerminalReport(t *testing.T) {
	now := time.Unix(0, 0)
	g, err := New("s", "obj", modelref.Selection{}, Budget{MaxTurns: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	g.Block(ReasonBlockedByModel, "need a credential", now)
	g.RecordTurn(TurnRecord{
		SessionID: "s", LeaseID: "lease", RunID: "run_1", Outcome: execution.OutcomeCompleted,
		CostUSD: 0.25, Steps: 2, CompletedAt: now.Add(time.Second),
	})
	if g.Status != StatusBlocked || g.Reason != (Reason{Cause: ReasonBlockedByModel, Detail: "need a credential"}) {
		t.Fatalf("status/reason after terminal record = %q/%+v", g.Status, g.Reason)
	}
	if g.Used != (Usage{Turns: 1, CostUSD: 0.25, Steps: 2}) {
		t.Fatalf("usage after terminal record = %+v", g.Used)
	}
}

func TestTransitions(t *testing.T) {
	now := time.Unix(0, 0)
	g, _ := New("s", "obj", modelref.Selection{}, Budget{}, now)

	g.AddTurn(0.5, 2, now)
	g.AddTurn(0.25, 1, now)
	if g.Used.Turns != 2 || g.Used.CostUSD != 0.75 || g.Used.Steps != 3 {
		t.Fatalf("usage accumulation = %+v", g.Used)
	}

	g.Block(ReasonTurnBudgetReached, "", now)
	if g.Status != StatusBlocked || g.Reason != (Reason{Cause: ReasonTurnBudgetReached}) {
		t.Fatalf("Block = (%q, %+v)", g.Status, g.Reason)
	}
	g.Resume(now)
	if g.Status != StatusActive || g.Reason != (Reason{}) {
		t.Fatalf("Resume = (%q, %+v), want (active, zero reason)", g.Status, g.Reason)
	}
	g.Pause(ReasonStoppedByUser, "", now)
	if g.Status != StatusPaused || g.Reason != (Reason{Cause: ReasonStoppedByUser}) {
		t.Fatalf("Pause = (%q, %+v)", g.Status, g.Reason)
	}
}
