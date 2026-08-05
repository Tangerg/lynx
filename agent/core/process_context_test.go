package core_test

import (
	"context"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/tool"
)

func TestProcessContextOwnsActionToolGroups(t *testing.T) {
	configured := []string{"research"}
	var resolved [][]string
	process := core.NewProcessContext(core.ProcessContextConfig{
		ActionToolGroups: configured,
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
