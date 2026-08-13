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

func TestPickerCancelsAStalePointerGesture(t *testing.T) {
	tests := []struct {
		name      string
		interrupt func(*picker[string], *headless.Root, image.Point)
	}{
		{
			name: "different button release",
			interrupt: func(_ *picker[string], root *headless.Root, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.MouseUp, Button: input.ButtonRight})
			},
		},
		{
			name: "different button press",
			interrupt: func(_ *picker[string], root *headless.Root, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.MouseDown, Button: input.ButtonRight})
			},
		},
		{
			name: "focus loss",
			interrupt: func(picker *picker[string], _ *headless.Root, _ image.Point) {
				picker.Focus(false)
				picker.Focus(true)
			},
		},
		{
			name: "catalog replacement",
			interrupt: func(picker *picker[string], _ *headless.Root, _ image.Point) {
				picker.SetItems([]string{"replacement first", "replacement second", "replacement third"})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			picked := ""
			picker := newPicker(kit.Dark(), kit.Unicode(), "search",
				func(value string) string { return value },
				func(string) string { return "" },
				func(value string) { picked = value },
			)
			picker.SetItems([]string{"first", "second", "third"})
			picker.Focus(true)
			root := headless.NewRoot(picker)
			root.Draw(grid.NewSurface(40, 7).View())
			second := pickerPoint(picker, 1)

			root.Handle(input.Mouse{Pos: second, Action: input.MouseDown, Button: input.ButtonLeft})
			test.interrupt(picker, root, second)
			root.Handle(input.Mouse{Pos: second, Action: input.MouseUp, Button: input.ButtonLeft})
			if picked != "" {
				t.Fatalf("stale pointer gesture picked %q", picked)
			}
		})
	}
}

func TestPickerDoesNotCommitAPointerGestureInterruptedByNavigation(t *testing.T) {
	tests := []struct {
		name      string
		interrupt func(*headless.Root, image.Point)
	}{
		{
			name: "keyboard",
			interrupt: func(root *headless.Root, _ image.Point) {
				root.Handle(input.Key{Code: input.Down})
				root.Handle(input.Key{Code: input.Up})
			},
		},
		{
			name: "wheel",
			interrupt: func(root *headless.Root, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.WheelDown})
				root.Handle(input.Mouse{Pos: point, Action: input.WheelUp})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			picked := ""
			picker := newPicker(kit.Dark(), kit.Unicode(), "search",
				func(value string) string { return value },
				func(string) string { return "" },
				func(value string) { picked = value },
			)
			picker.SetItems([]string{"first", "second", "third", "fourth", "fifth", "sixth", "seventh"})
			root := headless.NewRoot(picker)
			root.Draw(grid.NewSurface(40, 7).View())
			second := pickerPoint(picker, 1)

			root.Handle(input.Mouse{Pos: second, Action: input.MouseDown, Button: input.ButtonLeft})
			test.interrupt(root, second)
			root.Handle(input.Mouse{Pos: second, Action: input.MouseUp, Button: input.ButtonLeft})
			if picked != "" {
				t.Fatalf("interrupted pointer gesture picked %q", picked)
			}
		})
	}
}

func pickerPoint[T any](picker *picker[T], row int) image.Point {
	area := picker.areas.Value().list
	return image.Pt(area.Min.X+1, area.Min.Y+row)
}
