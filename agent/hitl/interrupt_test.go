package hitl

import "testing"

type fakeHalt struct {
	abort bool
}

func (f fakeHalt) Error() string { return "fake halt" }
func (f fakeHalt) Abort() bool   { return f.abort }

func TestIsInterrupt(t *testing.T) {
	if IsInterrupt(fakeHalt{abort: true}) {
		t.Fatal("expected aborting halt to be treated as non-resumeable")
	}
	if !IsInterrupt(fakeHalt{abort: false}) {
		t.Fatal("expected non-aborting halt to be treated as interrupt")
	}
	if IsInterrupt(&InterruptError{}) != true {
		t.Fatal("InterruptError should be recognized as resumeable interrupt")
	}
}
