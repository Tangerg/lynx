package goal

import (
	"testing"
	"time"
)

func TestNewValidates(t *testing.T) {
	now := time.Unix(0, 0)
	if _, err := New("", "obj", "", "", Budget{}, now); err == nil {
		t.Fatal("empty session should error")
	}
	if _, err := New("s", "", "", "", Budget{}, now); err == nil {
		t.Fatal("empty objective should error")
	}
	g, err := New("s", "obj", "p", "m", Budget{MaxTurns: 3}, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if g.Status != StatusActive {
		t.Fatalf("new goal status = %q, want active", g.Status)
	}
}

func TestBudgetExceeded(t *testing.T) {
	tests := []struct {
		name   string
		budget Budget
		used   Usage
		axis   string
		want   bool
	}{
		{"unbounded", Budget{}, Usage{Turns: 100, CostUSD: 999, Steps: 999}, "", false},
		{"under", Budget{MaxTurns: 5}, Usage{Turns: 4}, "", false},
		{"turns", Budget{MaxTurns: 5}, Usage{Turns: 5}, "turn", true},
		{"cost", Budget{MaxCostUSD: 1.0}, Usage{CostUSD: 1.0}, "cost", true},
		{"steps", Budget{MaxSteps: 10}, Usage{Steps: 11}, "step", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			axis, ok := tt.budget.Exceeded(tt.used)
			if ok != tt.want || axis != tt.axis {
				t.Fatalf("Exceeded = (%q, %v), want (%q, %v)", axis, ok, tt.axis, tt.want)
			}
		})
	}
}

func TestTransitions(t *testing.T) {
	now := time.Unix(0, 0)
	g, _ := New("s", "obj", "", "", Budget{}, now)

	g.AddTurn(0.5, 2, now)
	g.AddTurn(0.25, 1, now)
	if g.Used.Turns != 2 || g.Used.CostUSD != 0.75 || g.Used.Steps != 3 {
		t.Fatalf("usage accumulation = %+v", g.Used)
	}

	g.Block("budget", now)
	if g.Status != StatusBlocked || g.Reason != "budget" {
		t.Fatalf("Block = (%q, %q)", g.Status, g.Reason)
	}
	g.Resume(now)
	if g.Status != StatusActive || g.Reason != "" {
		t.Fatalf("Resume = (%q, %q), want (active, \"\")", g.Status, g.Reason)
	}
	g.Pause("user stop", now)
	if g.Status != StatusPaused || g.Reason != "user stop" {
		t.Fatalf("Pause = (%q, %q)", g.Status, g.Reason)
	}
}
