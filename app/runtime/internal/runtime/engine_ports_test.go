package runtime

import (
	"errors"
	"io"
	"testing"
)

func TestRuntimeCloseUsesCloserPort(t *testing.T) {
	closer := &fakeRuntimeCloser{}
	rt := &Runtime{closer: closer}

	if err := rt.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	if !closer.closed {
		t.Fatal("closer port was not called")
	}
}

func TestRuntimeCloseIsIdempotentAndJoinsResourceErrors(t *testing.T) {
	engineErr := errors.New("engine close")
	resourceErr := errors.New("resource close")
	engine := &fakeRuntimeCloser{err: engineErr}
	resource := &fakeRuntimeCloser{err: resourceErr}
	rt := &Runtime{closer: engine, resources: []io.Closer{resource}}

	for range 2 {
		err := rt.Close()
		if !errors.Is(err, engineErr) || !errors.Is(err, resourceErr) {
			t.Fatalf("Close err = %v, want both close errors", err)
		}
	}
	if engine.calls != 1 || resource.calls != 1 {
		t.Fatalf("close calls = engine:%d resource:%d, want 1 each", engine.calls, resource.calls)
	}
}

type fakeRuntimeCloser struct {
	closed bool
	calls  int
	err    error
}

func (f *fakeRuntimeCloser) Close() error {
	f.closed = true
	f.calls++
	return f.err
}
