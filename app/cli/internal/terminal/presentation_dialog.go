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

func (lease *presentationLease) bind(active func() bool) {
	if active == nil {
		panic("terminal: presentation lease requires an activity predicate")
	}
	if lease.active != nil {
		panic("terminal: presentation lease is already bound")
	}
	lease.active = active
}

func (lease *presentationLease) renew() {
	lease.current++
	if lease.current == 0 {
		panic("terminal: presentation lease exhausted")
	}
}

func (lease *presentationLease) stage(frame headless.Frame) {
	lease.presented.Stage(frame, lease.current)
}

func (lease *presentationLease) acceptsInput() bool {
	if lease.active == nil {
		return true
	}
	return lease.active() && lease.current != 0 && lease.presented.Value() == lease.current
}

// presentationGuard is a decorator for reusable dialog bodies. It preserves
// the wrapped component's focus contract while owning the cross-frame input
// boundary in one place.
type presentationGuard struct {
	body  headless.Widget
	lease presentationLease
}

func (guard *presentationGuard) Draw(frame headless.Frame) {
	guard.lease.stage(frame)
	if guard.body != nil {
		guard.body.Draw(frame)
	}
}

func (guard *presentationGuard) Handle(event input.Event) bool {
	if !guard.lease.acceptsInput() {
		return true
	}
	interactive, ok := guard.body.(headless.Interactive)
	return ok && interactive.Handle(event)
}

func (guard *presentationGuard) Focus(has bool) {
	if focusable, ok := guard.body.(headless.Focusable); ok {
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

func (dialog *presentationDialog) Show() {
	if dialog == nil || dialog.dialog == nil || dialog.guard == nil {
		return
	}
	dialog.guard.lease.renew()
	dialog.dialog.Show()
}

func (dialog *presentationDialog) Dismiss() {
	if dialog != nil && dialog.dialog != nil {
		dialog.dialog.Dismiss()
	}
}

func (dialog *presentationDialog) Open() bool {
	return dialog != nil && dialog.dialog != nil && dialog.dialog.Open()
}

func (dialog *presentationDialog) Controller() *headless.Dialog {
	if dialog == nil || dialog.dialog == nil {
		return nil
	}
	return dialog.dialog.Controller()
}
