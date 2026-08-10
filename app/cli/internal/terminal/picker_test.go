package terminal

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func TestPickerClickCommitsOnlyAfterMatchingRelease(t *testing.T) {
	picked := ""
	picker := newPicker(kit.Dark(), kit.Unicode(), "search",
		func(value string) string { return value },
		func(string) string { return "" },
		func(value string) { picked = value },
	)
	picker.SetItems([]string{"first", "second", "third"})
	root := headless.NewRoot(picker)
	root.Draw(grid.NewSurface(40, 7).View())
	second := pickerPoint(picker, 1)

	if !root.Handle(input.Mouse{Pos: second, Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("picker press was not handled")
	}
	if picked != "" || picker.items.Selected() != 1 {
		t.Fatalf("press picked %q with selection %d", picked, picker.items.Selected())
	}
	if !root.Handle(input.Mouse{Pos: second, Action: input.MouseUp, Button: input.ButtonLeft}) {
		t.Fatal("picker release was not handled")
	}
	if picked != "second" {
		t.Fatalf("matching release picked %q", picked)
	}
}

func TestPickerDragChangesSelectionWithoutActivation(t *testing.T) {
	picked := ""
	picker := newPicker(kit.Dark(), kit.Unicode(), "search",
		func(value string) string { return value },
		func(string) string { return "" },
		func(value string) { picked = value },
	)
	picker.SetItems([]string{"first", "second", "third"})
	root := headless.NewRoot(picker)
	root.Draw(grid.NewSurface(40, 7).View())
	first, third := pickerPoint(picker, 0), pickerPoint(picker, 2)

	root.Handle(input.Mouse{Pos: first, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: third, Action: input.MouseDrag, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: third, Action: input.MouseUp, Button: input.ButtonLeft})
	if picked != "" {
		t.Fatalf("drag activated %q", picked)
	}
	if got := picker.items.Selected(); got != 2 {
		t.Fatalf("drag selection = %d, want 2", got)
	}
}

func pickerPoint[T any](picker *picker[T], row int) image.Point {
	area := picker.areas.Value().list
	return image.Pt(area.Min.X+1, area.Min.Y+row)
}
