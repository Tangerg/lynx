package terminal

import "time"

const confirmationWindow = 800 * time.Millisecond

type confirmationKind uint8

const (
	confirmClearDraft confirmationKind = iota + 1
	confirmQuit
)

// pressConfirmation owns short-lived, destructive double-press gestures. Only
// two presses of the same semantic action confirm; arming a different action
// replaces the previous one.
type pressConfirmation struct {
	kind     confirmationKind
	deadline time.Time
}

func (p *pressConfirmation) Confirm(kind confirmationKind, now time.Time) bool {
	if p.kind == kind && !now.After(p.deadline) {
		p.Reset()
		return true
	}
	p.kind = kind
	p.deadline = now.Add(confirmationWindow)
	return false
}

func (p *pressConfirmation) Armed(kind confirmationKind) bool {
	return p.kind == kind
}

func (p *pressConfirmation) Reset() {
	*p = pressConfirmation{}
}
