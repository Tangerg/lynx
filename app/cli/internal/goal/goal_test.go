package goal

import (
	"math"
	"strings"
	"testing"
)

func TestGoalLifecycleValuesRejectAmbiguousState(t *testing.T) {
	active := Goal{SessionID: "ses_1", Objective: "finish the task", Status: Active}
	if err := active.Validate(); err != nil {
		t.Fatal(err)
	}
	active.Reason = &Reason{Code: StoppedByUser}
	if err := active.Validate(); err == nil {
		t.Fatal("active goal with a stop reason was accepted")
	}
	paused := Goal{SessionID: "ses_1", Objective: "finish the task", Status: Paused}
	if err := paused.Validate(); err == nil {
		t.Fatal("paused goal without a reason was accepted")
	}
	completing := Goal{SessionID: "ses_1", Objective: "finish the task", Status: Completing}
	if err := completing.Validate(); err != nil {
		t.Fatal(err)
	}
	completing.Reason = &Reason{Code: RunNotCompleted}
	if err := completing.Validate(); err == nil {
		t.Fatal("completing goal with a stop reason was accepted")
	}
	if completing.Status.AllowsLifecycleCommands() || !active.Status.AllowsLifecycleCommands() {
		t.Fatal("goal lifecycle command policy does not distinguish settlement")
	}
	if err := (Start{SessionID: "ses_1", Objective: "finish", Budget: Budget{MaxRuns: -1}}).Validate(); err == nil {
		t.Fatal("negative goal budget was accepted")
	}
	if err := (Budget{MaxCostUSD: math.NaN()}).Validate(); err == nil {
		t.Fatal("NaN goal budget was accepted")
	}
	if err := (Usage{CostUSD: math.Inf(1)}).Validate(); err == nil {
		t.Fatal("infinite goal usage was accepted")
	}
	if err := (Start{SessionID: "ses_1", Objective: "finish", Provider: " anthropic", Model: "deep"}).Validate(); err == nil {
		t.Fatal("non-canonical model selection was accepted")
	}
}

func TestGoalStartResultMustFulfillTheCommand(t *testing.T) {
	start := Start{
		SessionID: "ses_1", Objective: "finish", Provider: "anthropic", Model: "deep",
		Budget: Budget{MaxRuns: 3, MaxCostUSD: 1.5, MaxSteps: 20},
	}
	valid := Goal{
		SessionID: start.SessionID, Objective: start.Objective, Status: Active,
		Provider: start.Provider, Model: start.Model, Budget: start.Budget,
	}
	if err := start.ValidateResult(valid); err != nil {
		t.Fatalf("valid start result: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Goal)
		want   string
	}{
		{name: "session", mutate: func(result *Goal) { result.SessionID = "ses_other" }, want: "session"},
		{name: "objective", mutate: func(result *Goal) { result.Objective = "ignored" }, want: "objective"},
		{name: "status", mutate: func(result *Goal) {
			result.Status = Paused
			result.Reason = &Reason{Code: StoppedByUser}
		}, want: "status"},
		{name: "model", mutate: func(result *Goal) { result.Model = "shallow" }, want: "model"},
		{name: "budget", mutate: func(result *Goal) { result.Budget.MaxRuns++ }, want: "budget"},
		{name: "usage", mutate: func(result *Goal) { result.Used.Runs = 1 }, want: "non-zero usage"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := valid
			test.mutate(&result)
			err := start.ValidateResult(result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResult error = %v, want %q", err, test.want)
			}
		})
	}
}
