package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestRetryClearsEffectConditions verifies that when an action fails and is
// retried, its declared effect conditions are reset before the next attempt,
// so a half-applied effect from the failed attempt doesn't leak forward.
// Mirrors embabel's AbstractAgentProcess retry behavior (effect conditions
// cleared when retryCount > 0).
//
// The "eff" action declares a "side_effect" Post condition, sets it true,
// then fails its first attempt. Without the reset, the second attempt would
// observe the stale true; with it, the condition reads false again.
func TestRetryClearsEffectConditions(t *testing.T) {
	var seen []bool
	failOnce := true

	a := agent.New(agent.AgentConfig{Name: "retry-eff", Actions: []agent.Action{agent.NewAction("eff", func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
		cond, _ := pc.Blackboard().Condition("side_effect")
		seen = append(seen, cond)
		pc.Blackboard().StoreCondition("side_effect", true)
		if failOnce {
			failOnce = false
			return wordCount{}, errors.New("boom")
		}
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{Effects: []string{"side_effect"}, Retry: core.RetryPolicy{MaxAttempts: 3, Safety: core.RetrySafetyIdempotent}})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})

	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(
		context.Background(), a,
		core.Input(word{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure=%v", proc.Status(), proc.Failure())
	}

	if len(seen) != 2 {
		t.Fatalf("expected 2 attempts (1 failure + 1 retry), got %d: %v", len(seen), seen)
	}
	if seen[0] {
		t.Fatalf("attempt 1 should see the condition unset, got true")
	}
	if seen[1] {
		t.Fatal("attempt 2 should see the effect condition CLEARED (false); " +
			"got true — the retry leaked a half-applied effect")
	}
}

func TestActionFailureRunsOnceByDefault(t *testing.T) {
	var attempts int
	a := agent.New(agent.AgentConfig{Name: "default-single-attempt", Actions: []agent.Action{agent.NewAction("side-effect", func(context.Context, *core.ProcessContext, word) (wordCount, error) {
		attempts++
		return wordCount{}, errors.New("failed after side effect")
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a,
		core.Input(word{Text: "lynx"}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || proc.Status() != core.StatusFailed {
		t.Fatalf("attempts=%d status=%s, want one failed attempt", attempts, proc.Status())
	}
	history := proc.History()
	if len(history) != 1 || history[0].Attempts != 1 {
		t.Fatalf("history = %#v, want one invocation with one attempt", history)
	}
}

func TestExplicitRetryPolicyHonorsMaxAttempts(t *testing.T) {
	var attempts int
	a := agent.New(agent.AgentConfig{Name: "explicit-retry", Actions: []agent.Action{agent.NewAction("idempotent", func(context.Context, *core.ProcessContext, word) (wordCount, error) {
		attempts++
		return wordCount{}, errors.New("retryable failure")
	}, core.ActionConfig{Retry: core.RetryPolicy{MaxAttempts: 3, Safety: core.RetrySafetyIdempotent}})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a,
		core.Input(word{Text: "lynx"}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || proc.Status() != core.StatusFailed {
		t.Fatalf("attempts=%d status=%s, want three failed attempts", attempts, proc.Status())
	}
	if history := proc.History(); len(history) != 1 || history[0].Attempts != 3 {
		t.Fatalf("history = %#v, want one invocation with three attempts", history)
	}
}
