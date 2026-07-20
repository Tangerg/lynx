package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

type namespacedAction struct {
	metadata core.ActionMetadata
}

func (a namespacedAction) Metadata() core.ActionMetadata { return a.metadata }

func (namespacedAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionSucceeded, nil
}

func TestWorldStateReadsNamespacedActionRunCondition(t *testing.T) {
	action := namespacedAction{metadata: core.ActionMetadata{
		Name: "checkout:authorize",
		Preconditions: core.ConditionSet{
			core.ActionRunConditionPrefix + "checkout:authorize": core.False,
		},
		Effects: core.ConditionSet{
			core.ActionRunConditionPrefix + "checkout:authorize": core.True,
		},
	}}
	domain, err := planning.NewDomain([]core.Action{action}, nil, nil)
	if err != nil {
		t.Fatalf("NewDomain: %v", err)
	}
	blackboard := newInMemoryBlackboard()
	blackboard.StoreCondition(action.metadata.RunCondition(), true)

	state := newWorldStateReader(domain, blackboard, nil).read(t.Context())
	if got := state.Conditions()[action.metadata.RunCondition()]; got != core.True {
		t.Fatalf("run condition = %v, want true", got)
	}
}
