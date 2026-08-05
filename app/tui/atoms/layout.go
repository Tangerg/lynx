package atoms

import (
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
)

// Sizing says how much of an axis a slot wants.
type Sizing struct {
	// Fixed is an exact number of rows or columns. It wins over everything else.
	Fixed int
	// Flex is a share of what is left after the fixed and measured slots have taken
	// theirs. Two slots with flex 1 and 2 split the remainder one third to two
	// thirds.
	Flex int
	// Measured asks the slot's widget how much it wants, which only a [Sized]
	// widget can answer.
	Measured bool
	// Min is a floor on a flex or measured slot, so a pane cannot be squeezed into
	// a size where it shows nothing useful.
	Min int
	// Max caps a measured slot, so content that grew without bound does not take
	// the whole screen.
	Max int
}

// Fixed is a slot of an exact size.
func Fixed(n int) Sizing { return Sizing{Fixed: n} }

// Flex is a slot taking a share of what is left.
func Flex(share int) Sizing { return Sizing{Flex: share} }

// Measured is a slot as big as its widget asks to be, within bounds. A zero
// maximum means no cap.
func Measured(minRows, maxRows int) Sizing {
	return Sizing{Measured: true, Min: minRows, Max: maxRows}
}

// Slot is one child of a stack: what to draw and how much room it gets.
type Slot struct {
	Widget Widget
	Size   Sizing
}

// Rows stacks widgets vertically down v, giving each the height its sizing asks
// for, and returns the view each one was drawn into.
//
// The order of business is measure, then arrange, then draw — the only order that
// works when a slot's size depends on its content and another slot's depends on
// what is left. Slots that end up with no height are drawn into an empty view
// rather than skipped: a widget's draw code runs every frame, and a widget that
// only breaks when it is squeezed to nothing breaks in front of the user.
func Rows(v grid.View, slots ...Slot) []grid.View {
	width, height := v.Size()
	heights := distribute(height, width, slots, measureHeight)

	views := make([]grid.View, len(slots))
	y := 0
	for i, slot := range slots {
		views[i] = v.Sub(grid.Rect(0, y, width, heights[i]))
		if slot.Widget != nil {
			slot.Widget.Draw(views[i])
		}
		y += heights[i]
	}
	return views
}

// Columns is Rows across, for panes side by side.
func Columns(v grid.View, slots ...Slot) []grid.View {
	width, height := v.Size()
	widths := distribute(width, height, slots, measureWidth)

	views := make([]grid.View, len(slots))
	x := 0
	for i, slot := range slots {
		views[i] = v.Sub(grid.Rect(x, 0, widths[i], height))
		if slot.Widget != nil {
			slot.Widget.Draw(views[i])
		}
		x += widths[i]
	}
	return views
}

// measureHeight asks a widget how tall it wants to be at a width.
func measureHeight(w Widget, across int) int {
	if sized, ok := w.(Sized); ok {
		return sized.Height(across)
	}
	return 0
}

// measureWidth has no counterpart to ask: nothing here reports a width for a
// height, and a column whose width depends on its content is a table's problem,
// solved with fixed and flex sizing instead.
func measureWidth(Widget, int) int { return 0 }

// distribute divides total along one axis among slots.
func distribute(total, across int, slots []Slot, measure func(Widget, int) int) []int {
	sizes := make([]int, len(slots))
	left := max(total, 0)
	flex := 0

	// Fixed and measured slots take theirs first: both are stating a need, and the
	// flexible ones exist to absorb whatever is left over.
	for i, slot := range slots {
		switch {
		case slot.Size.Fixed > 0:
			sizes[i] = min(slot.Size.Fixed, left)
		case slot.Size.Measured:
			want := measure(slot.Widget, across)
			want = max(want, slot.Size.Min)
			if slot.Size.Max > 0 {
				want = min(want, slot.Size.Max)
			}
			sizes[i] = min(want, left)
		default:
			flex += max(slot.Size.Flex, 0)
			continue
		}
		left -= sizes[i]
	}
	if flex == 0 {
		return sizes
	}

	// Shares of the remainder, with the rounding remainder going to the last
	// flexible slot rather than being lost: a row that vanished would leave a gap
	// the user can see.
	remainder := left
	lastFlex := -1
	for i, slot := range slots {
		share := max(slot.Size.Flex, 0)
		if share == 0 && slot.Size.Fixed <= 0 && !slot.Size.Measured {
			continue
		}
		if share == 0 {
			continue
		}
		sizes[i] = max(remainder*share/flex, slot.Size.Min)
		lastFlex = i
	}
	used := 0
	for i, slot := range slots {
		if slot.Size.Flex > 0 {
			used += sizes[i]
		}
	}
	if lastFlex >= 0 && used < remainder {
		sizes[lastFlex] += remainder - used
	}
	return sizes
}
