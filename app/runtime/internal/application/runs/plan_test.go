package runs

import (
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
)

func testPlanState(t *testing.T, snapshot plan.Snapshot) plan.State {
	t.Helper()
	state, err := plan.Restore(snapshot)
	if err != nil {
		t.Fatalf("restore test Plan: %v", err)
	}
	return state
}
