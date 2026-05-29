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

	a := agent.New("retry-eff").
		Actions(agent.NewAction("eff",
			func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
				cond, _ := pc.Blackboard.Condition("side_effect")
				seen = append(seen, cond)

				// Half-apply the effect, then fail the first attempt so the
				// retry path runs.
				pc.Blackboard.SetCondition("side_effect", true)
				if failOnce {
					failOnce = false
					return wordCount{}, errors.New("boom")
				}
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{
				Post: []string{"side_effect"},
				QoS:  core.ActionQoS{MaxAttempts: 3},
			})).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "counted"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, a)

	proc, err := platform.RunAgent(
		context.Background(), a,
		map[string]any{core.DefaultBindingName: word{Text: "lynx"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
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
