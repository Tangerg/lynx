package panicerr

import (
	"errors"
	"testing"
)

func TestNewPreservesErrorCause(t *testing.T) {
	cause := errors.New("sentinel")
	err := New("worker panicked", cause)

	if !errors.Is(err, cause) {
		t.Fatalf("New() error = %v, want wrapped cause", err)
	}
	if got, want := err.Error(), "worker panicked: sentinel"; got != want {
		t.Fatalf("New() error = %q, want %q", got, want)
	}
}

func TestNewFormatsNonErrorValue(t *testing.T) {
	err := New("worker panicked", struct{ Code int }{Code: 7})
	if got, want := err.Error(), "worker panicked: {7}"; got != want {
		t.Fatalf("New() error = %q, want %q", got, want)
	}
}
