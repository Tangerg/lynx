package atoms

import (
	"image"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

func TestOverlayAnchors(t *testing.T) {
	for _, tc := range []struct {
		anchor Anchor
		want   image.Rectangle
	}{
		{TopLeft, grid.Rect(0, 0, 4, 2)},
		{Top, grid.Rect(3, 0, 4, 2)},
		{TopRight, grid.Rect(6, 0, 4, 2)},
		{Left, grid.Rect(0, 2, 4, 2)},
		{Middle, grid.Rect(3, 2, 4, 2)},
		{Right, grid.Rect(6, 2, 4, 2)},
		{BottomLeft, grid.Rect(0, 4, 4, 2)},
		{Bottom, grid.Rect(3, 4, 4, 2)},
		{BottomRight, grid.Rect(6, 4, 4, 2)},
	} {
		o := Overlay{Anchor: tc.anchor, Width: 4, Height: 2}
		if got := o.Area(grid.NewSurface(10, 6).View()); got != tc.want {
			t.Errorf("anchor %d = %v, want %v", tc.anchor, got, tc.want)
		}
	}
}

func TestOverlayIsClampedToTheSpaceItFloatsOver(t *testing.T) {
	// A dialog whose buttons are past the right margin is a dialog nobody can answer.
	o := Overlay{Width: 100, Height: 100, Margin: 1}
	got := o.Area(grid.NewSurface(10, 6).View())
	if got != grid.Rect(1, 1, 8, 4) {
		t.Fatalf("area = %v, want it clamped inside the margin", got)
	}
}

func TestOverlayWithNoSizeFillsWhatIsLeft(t *testing.T) {
	o := Overlay{Margin: 2}
	if got := o.Area(grid.NewSurface(20, 10).View()); got != grid.Rect(2, 2, 16, 6) {
		t.Fatalf("area = %v", got)
	}
}

func TestOverlayDrawsIntoWhereItSaidItWould(t *testing.T) {
	// Area and Draw have to agree, or a hit test a frame later answers about the wrong
	// place.
	o := Overlay{Anchor: Middle, Width: 4, Height: 1}
	s := grid.NewSurface(10, 3)
	area := o.Area(s.View())
	o.Draw(s.View()).Text(0, 0, "abcd", grid.Style{})

	rows := paint(10, 3, func(v grid.View) {
		o.Draw(v).Text(0, 0, "abcd", grid.Style{})
	})
	if !strings.Contains(rows[area.Min.Y], "abcd") {
		t.Fatalf("row %d = %q, want the layer on the row Area named", area.Min.Y, rows[area.Min.Y])
	}
	if got := strings.Index(rows[area.Min.Y], "abcd"); got != area.Min.X {
		t.Fatalf("layer starts at column %d, want %d", got, area.Min.X)
	}
}

func TestOverlayShadeRecedesWhatIsBehindWithoutErasingIt(t *testing.T) {
	// What is behind stays legible and simply recedes, which is what tells the reader it
	// is still there rather than gone.
	shade := grid.Style{Attr: grid.Dim}
	s := grid.NewSurface(8, 2)
	s.View().Text(0, 0, "behind", grid.Style{})
	Overlay{Width: 2, Height: 1, Shade: shade}.Draw(s.View())

	if got := s.CellAt(0, 0).Content; got != "b" {
		t.Fatalf("cell = %q, want what was behind still there", got)
	}
	if !s.CellAt(0, 0).Style.Attr.Has(grid.Dim) {
		t.Fatal("what is behind was not dimmed")
	}
}

func TestOverlayWithNowhereToGo(t *testing.T) {
	// None of this may panic: a layout collapses before it disappears.
	for _, size := range [][2]int{{0, 0}, {1, 1}, {2, 1}} {
		o := Overlay{Width: 4, Height: 4, Margin: 2, Shade: grid.Style{Attr: grid.Dim}}
		paint(size[0], size[1], func(v grid.View) { o.Draw(v) })
	}
}

// press builds a mouse event.
func press(x, y int, action input.MouseAction, button input.Button) input.Event {
	return input.Mouse{Pos: image.Pt(x, y), Action: action, Button: button}
}

func TestPointerTracksWhereItIs(t *testing.T) {
	var p Pointer
	if _, inside := p.Position(); inside {
		// A pointer that has never been reported is nowhere, not at the origin.
		t.Fatal("a fresh pointer claims to be somewhere")
	}
	if !p.Handle(press(3, 4, input.MouseMove, input.ButtonNone)) {
		t.Fatal("a mouse event was not taken")
	}
	at, inside := p.Position()
	if !inside || at != image.Pt(3, 4) {
		t.Fatalf("position = %v, %v", at, inside)
	}
	if p.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("the pointer consumed a keystroke")
	}
}

func TestPointerHover(t *testing.T) {
	var p Pointer
	box := grid.Rect(2, 2, 4, 2)
	p.Handle(press(3, 3, input.MouseMove, input.ButtonNone))
	if !p.Over(box) {
		t.Fatal("the pointer is inside the box but does not say so")
	}
	p.Handle(press(9, 9, input.MouseMove, input.ButtonNone))
	if p.Over(box) {
		t.Fatal("the pointer left the box and still says it is over it")
	}
}

func TestAClickCommitsOnReleaseOverTheTargetThatTookThePress(t *testing.T) {
	// A control that fired on the way down fires when the user was aiming at it and
	// changed their mind.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)

	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	p.Claim(box)
	if p.Clicked(box, input.ButtonLeft) {
		t.Fatal("the click fired on the way down")
	}
	if !p.Pressing(box) {
		t.Fatal("the control does not know it is being pushed")
	}
	p.Handle(press(1, 0, input.MouseUp, input.ButtonLeft))
	if !p.Clicked(box, input.ButtonLeft) {
		t.Fatal("the click never fired")
	}
}

func TestAPressDraggedAwayAndBackIsStillHeld(t *testing.T) {
	// It follows the press, not the pointer: the press was never released.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	p.Claim(box)

	p.Handle(press(9, 9, input.MouseDrag, input.ButtonLeft))
	if !p.Pressing(box) {
		t.Fatal("dragging away released a press that was still held")
	}
	p.Handle(press(1, 0, input.MouseDrag, input.ButtonLeft))
	p.Handle(press(1, 0, input.MouseUp, input.ButtonLeft))
	if !p.Clicked(box, input.ButtonLeft) {
		t.Fatal("coming back and releasing did not click")
	}
}

func TestAReleaseSomewhereElseIsNotAClick(t *testing.T) {
	// Which is how a user takes back a press they did not mean.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	p.Claim(box)
	p.Handle(press(9, 9, input.MouseUp, input.ButtonLeft))
	if p.Clicked(box, input.ButtonLeft) {
		t.Fatal("releasing away from the target still clicked it")
	}
	if p.Clicked(grid.Rect(8, 9, 4, 1), input.ButtonLeft) {
		t.Fatal("releasing over something else clicked that instead")
	}
}

func TestAPressBelongsToOneTarget(t *testing.T) {
	// Two overlapping regions must not both answer the same press.
	var p Pointer
	outer := grid.Rect(0, 0, 10, 4)
	inner := grid.Rect(1, 1, 4, 1)
	p.Handle(press(2, 1, input.MouseDown, input.ButtonLeft))
	p.Claim(inner)
	p.Claim(outer)
	p.Handle(press(2, 1, input.MouseUp, input.ButtonLeft))

	if p.Clicked(outer, input.ButtonLeft) {
		t.Fatal("the region that did not take the press answered the click")
	}
	if !p.Clicked(inner, input.ButtonLeft) {
		t.Fatal("the region that took the press did not answer the click")
	}
}

func TestAClickIsAnsweredOnce(t *testing.T) {
	// A widget asking twice in one frame, or two widgets asking in turn, must not both
	// act on the same click.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	p.Claim(box)
	p.Handle(press(1, 0, input.MouseUp, input.ButtonLeft))

	if !p.Clicked(box, input.ButtonLeft) {
		t.Fatal("the click never fired")
	}
	if p.Clicked(box, input.ButtonLeft) {
		t.Fatal("the same click fired twice")
	}
}

func TestAClickIsTheButtonThatWasPressed(t *testing.T) {
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonRight))
	p.Claim(box)
	p.Handle(press(1, 0, input.MouseUp, input.ButtonRight))
	if p.Clicked(box, input.ButtonLeft) {
		t.Fatal("a right press answered a left click")
	}
	if !p.Clicked(box, input.ButtonRight) {
		t.Fatal("the right click never fired")
	}
}

func TestLeavingTheInterfaceEndsHoverAndAnyPress(t *testing.T) {
	// A hover left highlighted under an unfocused window looks like the interface is
	// still live.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	p.Claim(box)

	p.Left()
	if p.Over(box) {
		t.Fatal("still hovering after the pointer left")
	}
	if p.Pressing(box) {
		t.Fatal("still holding a press after the pointer left")
	}
}

func TestAnUnclaimedPressPushesWhateverIsUnderIt(t *testing.T) {
	// Nothing has been drawn since the press, so the first frame after it is where a
	// control finds out it was pushed.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	if !p.Pressing(box) {
		t.Fatal("a control under an unclaimed press does not draw as pushed")
	}
	if p.Pressing(grid.Rect(8, 8, 2, 1)) {
		t.Fatal("a control nowhere near the press draws as pushed")
	}
}
