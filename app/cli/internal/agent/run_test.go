package agent

import (
	"math"
	"testing"
)

func TestRunLifecycleShape(t *testing.T) {
	running := runningRun("seg_1")
	if err := running.Validate(); err != nil {
		t.Fatal(err)
	}
	waiting := running
	waiting.Status, waiting.ActiveSegmentID = RunStatusWaiting, ""
	if err := waiting.Validate(); err != nil {
		t.Fatal(err)
	}
	finished := waiting
	finished.Status, finished.Outcome = RunStatusFinished, Outcome{Status: OutcomeCompleted}
	if err := finished.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRunOptionsValidateBounds(t *testing.T) {
	temperature, topP, maxTokens := 0.7, 0.9, int64(4096)
	options := RunOptions{
		Provider: "mock", Model: "balanced",
		Limits:     RunLimits{MaxSteps: 20, MaxBudgetUSD: 3},
		Generation: GenerationParams{Temperature: &temperature, TopP: &topP, MaxTokens: &maxTokens, Stop: []string{"END"}},
	}
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := 3.0
	options.Generation.Temperature = &bad
	if err := options.Validate(); err == nil {
		t.Fatal("invalid temperature was accepted")
	}
}

func TestOutcomeValidationMatchesRuntimeUnion(t *testing.T) {
	valid := []Outcome{
		{Status: OutcomeCompleted},
		{Status: OutcomeTimedOut, Error: "deadline exceeded"},
		{Status: OutcomeFailed, Error: "provider failed"},
		{Status: OutcomeLost, Error: "executor disappeared"},
		{Status: OutcomeMaxSteps, Detail: "20 / 20 steps"},
		{Status: OutcomeMaxBudget, Detail: "$2.00 / $2.00"},
		{Status: OutcomeCanceled, Detail: "user stopped"},
	}
	for _, outcome := range valid {
		if err := outcome.Validate(); err != nil {
			t.Fatalf("valid outcome %+v: %v", outcome, err)
		}
	}
	for _, outcome := range []Outcome{
		{Status: OutcomeTimedOut},
		{Status: OutcomeFailed, Detail: "wrong channel"},
		{Status: OutcomeCanceled, Error: "wrong channel"},
		{Status: OutcomeCompleted, Detail: "unexpected"},
	} {
		if err := outcome.Validate(); err == nil {
			t.Fatalf("invalid outcome %+v was accepted", outcome)
		}
	}
}

func TestUsagePreservesOptionalCostSemantics(t *testing.T) {
	knownZero := 0.0
	usage := Usage{CostUSD: &knownZero}
	if err := usage.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := usage.Clone()
	*usage.CostUSD = 1
	if cloned.CostUSD == nil || *cloned.CostUSD != 0 {
		t.Fatalf("cloned cost = %v", cloned.CostUSD)
	}

	invalid := math.NaN()
	if err := (Usage{CostUSD: &invalid}).Validate(); err == nil {
		t.Fatal("NaN cost was accepted")
	}
	if err := validateUsageProgress(Usage{CostUSD: &knownZero}, Usage{}); err == nil {
		t.Fatal("known cumulative cost became unknown")
	}
}
