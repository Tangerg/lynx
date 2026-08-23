package runtimehost

import "errors"

// openGuard owns resources only while Runtime construction is incomplete.
// Actions are registered immediately after acquisition and unwind in reverse
// order. A successful composition transfers every resource to Runtime by
// disarming the guard.
type openGuard struct {
	actions []func() error
	armed   bool
}

func newOpenGuard() *openGuard { return &openGuard{armed: true} }

func (guard *openGuard) Add(action func() error) {
	if action != nil {
		guard.actions = append(guard.actions, action)
	}
}

func (guard *openGuard) AddClose(action func()) {
	if action != nil {
		guard.Add(func() error { action(); return nil })
	}
}

func (guard *openGuard) Disarm() {
	guard.armed = false
	guard.actions = nil
}

func (guard *openGuard) Close() error {
	if !guard.armed {
		return nil
	}
	guard.armed = false
	var joined error
	for index := len(guard.actions) - 1; index >= 0; index-- {
		joined = errors.Join(joined, guard.actions[index]())
	}
	guard.actions = nil
	return joined
}
