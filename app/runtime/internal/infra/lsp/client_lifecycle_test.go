package lsp

import (
	"errors"
	"io"
	"testing"
)

func TestPipeRWCCloseJoinsBothPipeErrors(t *testing.T) {
	writeErr := errors.New("close stdin")
	readErr := errors.New("close stdout")
	pipe := &pipeRWC{
		in:  failingWriteCloser{err: writeErr},
		out: failingReadCloser{err: readErr},
	}
	err := pipe.Close()
	if !errors.Is(err, writeErr) || !errors.Is(err, readErr) {
		t.Fatalf("Close error = %v, want both pipe errors", err)
	}
}

type failingWriteCloser struct{ err error }

func (failingWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (f failingWriteCloser) Close() error              { return f.err }

type failingReadCloser struct{ err error }

func (failingReadCloser) Read([]byte) (int, error) { return 0, io.EOF }
func (f failingReadCloser) Close() error           { return f.err }
