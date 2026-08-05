package core_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/tool"
)

type typedNilProcessView struct{ core.ProcessView }
type typedNilProcessControl struct{ core.ProcessControl }
type typedNilBlackboard struct{ core.Blackboard }

func TestProcessContextOwnsActionToolRoles(t *testing.T) {
	configured := []string{"research"}
	var resolved [][]string
	process := core.NewProcessContext(core.ProcessContextConfig{
		ActionToolRoles: configured,
		ActionTools: func(_ context.Context, groups []string) ([]tool.Tool, error) {
			resolved = append(resolved, slices.Clone(groups))
			groups[0] = "mutated by resolver"
			return nil, nil
		},
	})

	configured[0] = "mutated by caller"
	for range 2 {
		if _, err := process.ActionTools(t.Context()); err != nil {
			t.Fatalf("ActionTools: %v", err)
		}
	}

	want := []string{"research"}
	for index, groups := range resolved {
		if !slices.Equal(groups, want) {
			t.Fatalf("resolver call %d groups = %v, want %v", index, groups, want)
		}
	}
}

func TestProcessContextNormalizesTypedNilCapabilities(t *testing.T) {
	var process *typedNilProcessView
	var control *typedNilProcessControl
	var blackboard *typedNilBlackboard
	context := core.NewProcessContext(core.ProcessContextConfig{
		Process: process, Control: control, Blackboard: blackboard,
	})
	if context.Process() != nil || context.Blackboard() != nil {
		t.Fatalf("typed-nil capabilities survived: process=%v blackboard=%v", context.Process(), context.Blackboard())
	}
	if err := context.Terminate("stop"); !errors.Is(err, core.ErrLifecycleControlUnavailable) {
		t.Fatalf("Terminate error = %v, want unavailable control", err)
	}
}
