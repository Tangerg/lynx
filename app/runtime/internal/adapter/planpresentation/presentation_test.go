package planpresentation

import (
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
)

func TestRenderUsesStableStepMarkers(t *testing.T) {
	steps := []plan.Step{
		{Description: "write tests", Status: plan.StatusCompleted},
		{Description: "ship it", Status: plan.StatusInProgress},
		{Description: "celebrate", Status: plan.StatusPending},
	}
	if got, want := Render(steps), "[x] write tests\n[~] ship it\n[ ] celebrate\n"; got != want {
		t.Fatalf("rendered Plan = %q, want %q", got, want)
	}
	if got := Render(nil); got != "" {
		t.Fatalf("empty Plan = %q, want empty", got)
	}
}
