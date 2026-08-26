package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
)

// presentationLease separates the semantic lifetime of a reusable control from
// the last complete frame that displayed it. Reopening the same dialog advances
// the lease, so input routed through an older frame is consumed without reaching
// the replacement presentation.
type presentationLease struct {
	current   uint64
	presented headless.Snapshot[uint64]
	active    func() bool
}

func (p *presentationLease) bind(active func() bool) {
	if active == nil {
		panic("terminal: presentation lease requires an activity predicate")
	}
	if p.active != nil {
		panic("terminal: presentation lease is already bound")
	}
	p.active = active
}

func (p *presentationLease) renew() {
	p.current++
	if p.current == 0 {
		panic("terminal: presentation lease exhausted")
	}
}

func (p *presentationLease) stage(frame headless.Frame) {
	p.presented.Stage(frame, p.current)
}

func (p *presentationLease) acceptsInput() bool {
	if p.active == nil {
		return true
	}
	return p.active() && p.current != 0 && p.presented.Value() == p.current
}

// presentationGuard is a decorator for reusable dialog bodies. It preserves
// the wrapped component's focus contract while owning the cross-frame input
// boundary in one place.
type presentationGuard struct {
	body  headless.Widget
	lease presentationLease
}

func (p *presentationGuard) Draw(frame headless.Frame) {
	p.lease.stage(frame)
	if p.body != nil {
		p.body.Draw(frame)
	}
}

func (p *presentationGuard) Handle(event input.Event) bool {
	if !p.lease.acceptsInput() {
		return true
	}
	interactive, ok := p.body.(headless.Interactive)
	return ok && interactive.Handle(event)
}

func (p *presentationGuard) Focus(has bool) {
	if focusable, ok := p.body.(headless.Focusable); ok {
		focusable.Focus(has)
	}
}

// presentationDialog gives a reusable kit dialog a distinct input identity for
// every Show call. It intentionally exposes only the controller operations the
// terminal needs, keeping the generation boundary impossible to bypass.
type presentationDialog struct {
	dialog *kit.Dialog
	guard  *presentationGuard
}

func newPresentationDialog(config kit.DialogConfig) *presentationDialog {
	guard := &presentationGuard{body: config.Body}
	config.Body = guard
	dialog := kit.NewDialog(config)
	guard.lease.bind(dialog.Open)
	return &presentationDialog{dialog: dialog, guard: guard}
}

func (p *presentationDialog) Show() {
	if p == nil || p.dialog == nil || p.guard == nil {
		return
	}
	p.guard.lease.renew()
	p.dialog.Show()
}

func (p *presentationDialog) Dismiss() {
	if p != nil && p.dialog != nil {
		p.dialog.Dismiss()
	}
}

func (p *presentationDialog) Open() bool {
	return p != nil && p.dialog != nil && p.dialog.Open()
}

func (p *presentationDialog) Controller() *headless.Dialog {
	if p == nil || p.dialog == nil {
		return nil
	}
	return p.dialog.Controller()
}
