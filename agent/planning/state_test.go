package planning_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

func TestStateApplyPreservesObservationTimestamp(t *testing.T) {
	state := planning.NewState(core.ConditionSet{"ready": core.False})
	applied := state.Apply(core.ConditionSet{"ready": core.True})

	if !applied.Timestamp().Equal(state.Timestamp()) {
		t.Fatalf("timestamps = %v/%v, want one observation time", state.Timestamp(), applied.Timestamp())
	}
	if state.Conditions()["ready"] != core.False || applied.Conditions()["ready"] != core.True {
		t.Fatalf("state/applied = %v/%v, want immutable false/true", state.Conditions(), applied.Conditions())
	}
}
