package atoms

import (
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

// List is a vertical list of one-row items with a selection.
//
// It is generic over the item so a list of sessions and a list of files are the same
// widget with different rows, and so nothing here has to know what an item is. The
// row is drawn by a function the caller supplies: a list that formatted its own
// items would be a list that had opinions about them.
//
// Selection and scrolling are separate concerns that have to agree: moving the
// selection past the edge of the window scrolls to keep it visible, because a
// selection the user cannot see is a selection they will act on by mistake.
type List[T any] struct {
	// Items are what the list shows.
	Items []T
	// Row draws one item. selected says whether it is the one under the cursor,
	// which the caller renders however it likes — a list does not know what
	// selected looks like in its surroundings.
	Row func(v grid.View, item T, selected bool)
	// Keys are the bindings the list answers.
	Keys ListKeys
	// Wrap moves the selection from the last item to the first and back. Off by
	// default: in a long list, wrapping loses the user's place.
	Wrap bool

	selected int
	scroll   Scroll
	// window is the last drawn height, which is what a page-sized move needs and
	// what only drawing can know.
	window int
}

// ListKeys are the keystrokes a list answers.
type ListKeys struct {
	Up, Down       Binding
	PageUp, PageDn Binding
	First, Last    Binding
}

// DefaultListKeys are the bindings a terminal list expects, including the pair of
// letters every reader's fingers already know.
func DefaultListKeys() ListKeys {
	return ListKeys{
		Up:     Binding{Key: input.Key{Code: input.Up}, Does: "up"},
		Down:   Binding{Key: input.Key{Code: input.Down}, Does: "down"},
		PageUp: Binding{Key: input.Key{Code: input.PageUp}, Does: "page up"},
		PageDn: Binding{Key: input.Key{Code: input.PageDown}, Does: "page down"},
		First:  Binding{Key: input.Key{Code: input.Home}, Does: "first"},
		Last:   Binding{Key: input.Key{Code: input.End}, Does: "last"},
	}
}

// Selected is the index under the cursor, or -1 for an empty list.
func (l *List[T]) Selected() int {
	if len(l.Items) == 0 {
		return -1
	}
	return l.clampIndex(l.selected)
}

// Current is the item under the cursor, and whether there was one.
func (l *List[T]) Current() (T, bool) {
	i := l.Selected()
	if i < 0 {
		var zero T
		return zero, false
	}
	return l.Items[i], true
}

// Select moves the cursor to an index, clamped to the list.
func (l *List[T]) Select(i int) {
	l.selected = l.clampIndex(i)
	l.reveal()
}

// Move shifts the selection by n items, wrapping only if asked to.
func (l *List[T]) Move(n int) {
	if len(l.Items) == 0 {
		return
	}
	next := l.selected + n
	if l.Wrap {
		size := len(l.Items)
		next = ((next % size) + size) % size
	}
	l.Select(next)
}

// SetItems replaces the contents, keeping the selection on the same index where
// that still exists.
//
// Keeping the index rather than the item: a list that is refreshed while the user is
// reading it should not jump, and following an item by identity would need this
// widget to know how to compare items, which is knowledge it has no business
// holding.
func (l *List[T]) SetItems(items []T) {
	l.Items = items
	l.selected = l.clampIndex(l.selected)
}

// Handle answers keys and the wheel, reporting whether it consumed the event.
func (l *List[T]) Handle(ev input.Event) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		switch mouse.Action {
		case input.WheelUp:
			l.scroll.By(-wheelRows)
			return true
		case input.WheelDown:
			l.scroll.By(wheelRows)
			return true
		default:
			return false
		}
	}
	keys := l.keys()
	page := max(l.window-1, 1)
	switch {
	case keys.Up.Matches(ev):
		l.Move(-1)
	case keys.Down.Matches(ev):
		l.Move(1)
	case keys.PageUp.Matches(ev):
		l.Move(-page)
	case keys.PageDn.Matches(ev):
		l.Move(page)
	case keys.First.Matches(ev):
		l.Select(0)
	case keys.Last.Matches(ev):
		l.Select(len(l.Items) - 1)
	default:
		return false
	}
	return true
}

// keys are the bindings to answer, filling in the ones a caller left unset.
//
// Lazily rather than in a constructor, because a list is a struct a caller fills in
// and so its zero value has to work: one that quietly ignored the arrow keys would
// look finished and not be.
func (l *List[T]) keys() ListKeys {
	if l.Keys == (ListKeys{}) {
		l.Keys = DefaultListKeys()
	}
	return l.Keys
}

// Height is one row per item, which is what a container needs to decide whether the
// list can have all the room it wants.
func (l *List[T]) Height(int) int { return len(l.Items) }

// Scroll exposes the position, for a scrollbar drawn beside the list.
func (l *List[T]) Scroll() *Scroll { return &l.scroll }

// Draw paints the visible items.
func (l *List[T]) Draw(v grid.View) {
	_, height := v.Size()
	l.window = height
	l.selected = l.clampIndex(l.selected)
	l.reveal()
	if l.Row == nil {
		return
	}
	selected := l.Selected()
	l.scroll.Rows(v, len(l.Items), func(row grid.View, index int) {
		l.Row(row, l.Items[index], index == selected)
	})
}

// reveal scrolls the least amount that brings the selection into the window.
func (l *List[T]) reveal() {
	if l.window <= 0 || len(l.Items) == 0 {
		return
	}
	l.scroll.Layout(len(l.Items), l.window)
	first := l.scroll.Offset()
	switch last := first + l.window - 1; {
	case l.selected < first:
		l.scroll.By(l.selected - first)
	case l.selected > last:
		l.scroll.By(l.selected - last)
	}
}

func (l *List[T]) clampIndex(i int) int {
	if len(l.Items) == 0 {
		return 0
	}
	return min(max(i, 0), len(l.Items)-1)
}
