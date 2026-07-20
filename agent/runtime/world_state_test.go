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
	metadata := core.ActionMetadata{Name: "checkout:authorize"}
	metadata.Preconditions = core.ConditionSet{metadata.RunCondition(): core.False}
	metadata.Effects = core.ConditionSet{metadata.RunCondition(): core.True}
	action := namespacedAction{metadata: metadata}
	domain, err := planning.NewDomain([]core.Action{action}, nil, nil)
	if err != nil {
		t.Fatalf("NewDomain: %v", err)
	}
	blackboard := newInMemoryBlackboard()
	blackboard.StoreCondition(action.metadata.RunCondition(), true)

	state, err := newWorldStateReader(domain, blackboard, nil).read(t.Context())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := state.Conditions()[action.metadata.RunCondition()]; got != core.True {
		t.Fatalf("run condition = %v, want true", got)
	}
}
