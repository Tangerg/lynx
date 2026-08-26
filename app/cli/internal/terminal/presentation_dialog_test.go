package terminal

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

type presentationProbe struct {
	handled int
	focused bool
}

func (*presentationProbe) Draw(headless.Frame) {}

func (p *presentationProbe) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() || key.Code != input.Enter {
		return false
	}
	p.handled++
	return true
}

func (p *presentationProbe) Focus(has bool) { p.focused = has }

func TestPresentationDialogRejectsInputUntilTheCurrentOpeningIsVisible(t *testing.T) {
	var stack headless.Stack
	probe := &presentationProbe{}
	dialog := newPresentationDialog(kit.DialogConfig{
		Stack: &stack, Theme: kit.Dark(), Glyphs: kit.Unicode(), Title: "Probe", Body: probe,
	})
	root := headless.NewRoot(&stack)
	surface := grid.NewSurface(40, 8)
	enter := input.Key{Code: input.Enter}

	dialog.Show()
	root.Draw(surface.View())
	if !probe.focused || !root.Handle(enter) || probe.handled != 1 {
		t.Fatalf("visible opening = focused %t, handled %d", probe.focused, probe.handled)
	}

	dialog.Dismiss()
	if probe.focused || !root.Handle(enter) || probe.handled != 1 {
		t.Fatalf("dismissed opening accepted input: focused %t, handled %d", probe.focused, probe.handled)
	}

	dialog.Show()
	if !dialog.Open() || !root.Handle(enter) || probe.handled != 1 {
		t.Fatalf("undrawn replacement accepted input: open %t, handled %d", dialog.Open(), probe.handled)
	}
	root.Draw(surface.View())
	if !root.Handle(enter) || probe.handled != 2 {
		t.Fatalf("visible replacement handled %d inputs, want 2", probe.handled)
	}
}

func TestPresentationDialogShowInvalidatesAnOpenPresentation(t *testing.T) {
	var stack headless.Stack
	probe := &presentationProbe{}
	dialog := newPresentationDialog(kit.DialogConfig{
		Stack: &stack, Theme: kit.Dark(), Glyphs: kit.Unicode(), Title: "Probe", Body: probe,
	})
	root := headless.NewRoot(&stack)
	surface := grid.NewSurface(40, 8)
	enter := input.Key{Code: input.Enter}

	dialog.Show()
	root.Draw(surface.View())
	dialog.Show()
	if !root.Handle(enter) || probe.handled != 0 {
		t.Fatalf("refreshed but undrawn presentation handled %d inputs", probe.handled)
	}
	root.Draw(surface.View())
	if !root.Handle(enter) || probe.handled != 1 {
		t.Fatalf("drawn refresh handled %d inputs, want 1", probe.handled)
	}
}
