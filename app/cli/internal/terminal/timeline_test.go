package terminal

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestTimelineGroupsDescendantsBeneathNewestRoots(t *testing.T) {
	runs := []agent.Run{
		{ID: "run_old"},
		{ID: "run_new"},
		{ID: "run_child", Lineage: agent.RunLineage{SpawnedByBlockID: "spawn", ParentRunID: "run_new", RootRunID: "run_new"}},
		{ID: "run_grandchild", Lineage: agent.RunLineage{SpawnedByBlockID: "nested", ParentRunID: "run_child", RootRunID: "run_new"}},
	}
	entries := buildTimelineEntries(runs)
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.Run.ID)
	}
	if want := []string{"run_new", "run_child", "run_grandchild", "run_old"}; !slices.Equal(ids, want) {
		t.Fatalf("timeline run order = %v, want %v", ids, want)
	}
	if entries[0].RootPosition != 2 || entries[0].RootTotal != 2 || entries[0].Depth != 0 ||
		entries[1].RootPosition != 2 || entries[1].Depth != 1 || entries[2].Depth != 2 ||
		entries[3].RootPosition != 1 || entries[3].Depth != 0 {
		t.Fatalf("timeline entries = %+v", entries)
	}
}

func TestTimelineCommandInterruptsAPendingPickerClick(t *testing.T) {
	jumped, forked := 0, 0
	pane := newTimelinePane(kit.Dark(), kit.Unicode(),
		func(timelineEntry) { jumped++ },
		func(timelineEntry) { forked++ },
	)
	pane.SetRuns([]agent.Run{{ID: "one"}, {ID: "two"}})
	pane.Focus(true)
	root := headless.NewRoot(pane)
	root.Draw(grid.NewSurface(80, 20).View())
	first := pickerPoint(pane.picker, 0)

	root.Handle(input.Mouse{Pos: first, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt})
	root.Handle(input.Mouse{Pos: first, Action: input.MouseUp, Button: input.ButtonLeft})

	if forked != 1 {
		t.Fatalf("fork command ran %d times, want 1", forked)
	}
	if jumped != 0 {
		t.Fatalf("release after fork jumped %d times", jumped)
	}
}
