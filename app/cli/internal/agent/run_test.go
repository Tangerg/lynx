package agent

import (
	"math"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/failure"
)

func TestRunLifecycleShape(t *testing.T) {
	running := runningRun("seg_1")
	running.CreatedAt = time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	running.Contract = &RunContract{
		RequiredFeatures: []RunFeature{RunFeatureSubagents},
		InteractionKinds: []InteractionKind{InteractionApproval, InteractionQuestion},
	}
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
	finished.FinishedAt = finished.CreatedAt.Add(time.Second)
	if err := finished.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := running
	invalid.FinishedAt = invalid.CreatedAt.Add(time.Second)
	if err := invalid.Validate(); err == nil {
		t.Fatal("running run with a finish time was accepted")
	}
	cloned := running.Clone()
	cloned.Contract.InteractionKinds[0] = InteractionQuestion
	if running.Contract.InteractionKinds[0] != InteractionApproval || running.Equal(cloned) {
		t.Fatal("run clone shares its negotiated contract")
	}
	invalidContract := running.Clone()
	invalidContract.Contract.RequiredFeatures = append(invalidContract.Contract.RequiredFeatures, RunFeatureSubagents)
	if err := invalidContract.Validate(); err == nil {
		t.Fatal("run accepted a duplicate negotiated feature")
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

func TestRunOptionsEqualPreservesOptionalGenerationSemantics(t *testing.T) {
	zero := 0.0
	left := RunOptions{Provider: "deepseek", Model: "v4", Generation: GenerationParams{Temperature: &zero, Stop: []string{"END"}}}
	if !left.Equal(left.Clone()) {
		t.Fatal("cloned options are not equal")
	}
	right := left.Clone()
	right.Generation.Temperature = nil
	if left.Equal(right) {
		t.Fatal("explicit zero temperature equals an omitted temperature")
	}
	right = left.Clone()
	right.Generation.Stop[0] = "STOP"
	if left.Equal(right) {
		t.Fatal("different stop sequences are equal")
	}
}

func TestOutcomeValidationMatchesRuntimeUnion(t *testing.T) {
	problem := &failure.Problem{Type: "rate_limited", Detail: "deadline exceeded", RetryAfterSeconds: 2}
	valid := []Outcome{
		{Status: OutcomeCompleted},
		{Status: OutcomeTimedOut, Problem: problem},
		{Status: OutcomeFailed, Problem: &failure.Problem{Type: "provider_failed", Detail: "provider failed"}},
		{Status: OutcomeLost, Problem: &failure.Problem{Type: "run_lost", Detail: "executor disappeared"}},
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
		{Status: OutcomeCanceled, Problem: problem},
		{Status: OutcomeCompleted, Detail: "unexpected"},
		{Status: OutcomeCompleted, Problem: problem},
		{Status: OutcomeFailed, Problem: &failure.Problem{}},
	} {
		if err := outcome.Validate(); err == nil {
			t.Fatalf("invalid outcome %+v was accepted", outcome)
		}
	}
	cloned := valid[1].Clone()
	cloned.Problem.Detail = "mutated"
	if valid[1].Equal(cloned) || !valid[1].Equal(valid[1].Clone()) {
		t.Fatal("outcome problem is not value-owned")
	}
}

func TestUsagePreservesOptionalCostSemantics(t *testing.T) {
	knownZero, modelCost := 0.0, 0.25
	usage := Usage{
		CostUSD: &knownZero, Steps: 3,
		ByModel: map[string]ModelUsage{"deepseek/v4": {InputTokens: 12, CostUSD: &modelCost}},
	}
	if err := usage.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := usage.Clone()
	*usage.CostUSD = 1
	model := usage.ByModel["deepseek/v4"]
	*model.CostUSD = 2
	usage.ByModel["deepseek/v4"] = model
	if cloned.CostUSD == nil || *cloned.CostUSD != 0 || cloned.ByModel["deepseek/v4"].CostUSD == nil ||
		*cloned.ByModel["deepseek/v4"].CostUSD != 0.25 || !cloned.Equal(cloned.Clone()) || cloned.Empty() {
		t.Fatalf("cloned usage = %+v", cloned)
	}

	invalid := math.NaN()
	if err := (Usage{CostUSD: &invalid}).Validate(); err == nil {
		t.Fatal("NaN cost was accepted")
	}
	if err := validateUsageProgress(Usage{CostUSD: &knownZero}, Usage{}); err == nil {
		t.Fatal("known cumulative cost became unknown")
	}
	if err := (Usage{Steps: -1}).Validate(); err == nil {
		t.Fatal("negative step usage was accepted")
	}
	if err := (Usage{ByModel: map[string]ModelUsage{"": {}}}).Validate(); err == nil {
		t.Fatal("empty model attribution key was accepted")
	}
	if err := validateUsageProgress(
		Usage{Steps: 3, ByModel: map[string]ModelUsage{"deepseek/v4": {InputTokens: 12}}},
		Usage{Steps: 2},
	); err == nil {
		t.Fatal("step or per-model usage regression was accepted")
	}
}
