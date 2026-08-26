package failure

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type carryingError struct{ problem *Problem }

func (c carryingError) Error() string     { return "failed" }
func (c carryingError) Failure() *Problem { return c.problem.Clone() }

func TestProblemOwnsAndPresentsRecoveryMetadata(t *testing.T) {
	t.Parallel()

	problem := &Problem{
		Type: "capability_not_negotiated", Detail: "additional declarations required",
		DocURL: "https://docs.example/capabilities", RetryAfterSeconds: 2,
		RequiredCapabilities: []CapabilityRequirement{{Kind: RequirementFeature, Name: "subagents"}},
		ActiveRun:            &ActiveRun{RunID: "run_1", Status: "waiting"},
		Errors:               []FieldError{{Field: "features", Detail: "subagents is absent"}},
	}
	if err := problem.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"additional declarations required", "retry after 2s", "docs.example", "feature:subagents", "run_1 (waiting)", "features: subagents is absent"} {
		if !strings.Contains(problem.String(), want) {
			t.Fatalf("Problem.String() omitted %q: %s", want, problem)
		}
	}

	clone := problem.Clone()
	clone.RequiredCapabilities[0].Name = "mutated"
	clone.ActiveRun.RunID = "run_2"
	clone.Errors[0].Detail = "mutated"
	if problem.Equal(clone) || problem.RequiredCapabilities[0].Name != "subagents" || problem.ActiveRun.RunID != "run_1" || problem.Errors[0].Detail != "subagents is absent" {
		t.Fatalf("Clone retained caller-owned state: source=%+v clone=%+v", problem, clone)
	}
	if !problem.Equal(problem.Clone()) || (*Problem)(nil).Clone() != nil || !(*Problem)(nil).Equal(nil) {
		t.Fatal("problem clone/equality identity is broken")
	}
}

func TestProblemRejectsMalformedStructuredLeaves(t *testing.T) {
	t.Parallel()

	tests := []Problem{
		{},
		{Type: "rate_limited", RetryAfterSeconds: -1},
		{Type: "capability_not_negotiated", RequiredCapabilities: []CapabilityRequirement{{Kind: "unknown", Name: "x"}}},
		{Type: "capability_not_negotiated", RequiredCapabilities: []CapabilityRequirement{{Kind: RequirementFeature}}},
		{Type: "session_has_active_run", ActiveRun: &ActiveRun{Status: "running"}},
		{Type: "session_has_active_run", ActiveRun: &ActiveRun{RunID: "run_1", Status: "queued"}},
		{Type: "invalid_params", Errors: []FieldError{{Field: "provider"}}},
	}
	for _, problem := range tests {
		if err := problem.Validate(); err == nil {
			t.Fatalf("Validate accepted malformed problem: %+v", problem)
		}
	}
}

func TestFromErrorFindsAndOwnsNestedFailure(t *testing.T) {
	t.Parallel()

	source := &Problem{Type: "rate_limited", Detail: "wait"}
	problem, ok := FromError(fmt.Errorf("outer: %w", carryingError{problem: source}))
	if !ok || problem == nil || !problem.Equal(source) {
		t.Fatalf("FromError = (%+v, %v)", problem, ok)
	}
	problem.Detail = "mutated"
	if source.Detail != "wait" {
		t.Fatal("FromError returned carrier-owned state")
	}
	if problem, ok := FromError(errors.New("ordinary")); ok || problem != nil {
		t.Fatalf("ordinary error produced a problem: (%+v, %v)", problem, ok)
	}
}
