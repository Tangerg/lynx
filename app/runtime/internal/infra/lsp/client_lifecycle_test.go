package lsp

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
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

func TestCloseUnstartedPipesClosesPartialSetupAndPreservesErrors(t *testing.T) {
	cause := errors.New("start failed")
	readErr := errors.New("close stdout")
	stdout := &trackedReadCloser{err: readErr}

	err := closeUnstartedPipes("test-lsp", &pipeRWC{out: stdout}, cause)
	if !stdout.closed {
		t.Fatal("stdout pipe was not closed")
	}
	if !errors.Is(err, cause) || !errors.Is(err, readErr) {
		t.Fatalf("closeUnstartedPipes error = %v, want launch and cleanup errors", err)
	}
}

func TestKillAndJoinProcessReapsWaiter(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestLSPProcessHelper$")
	cmd.Env = append(os.Environ(), "LYNX_LSP_PROCESS_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	wait := make(chan error, 1)
	go func() {
		wait <- cmd.Wait()
		close(wait)
	}()

	if err := killAndJoinProcess("test", cmd.Process, wait); err != nil {
		t.Fatalf("killAndJoinProcess: %v", err)
	}
	if _, open := <-wait; open {
		t.Fatal("process waiter was not joined")
	}
}

func TestLSPProcessHelper(t *testing.T) {
	if os.Getenv("LYNX_LSP_PROCESS_HELPER") != "1" {
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}

type failingWriteCloser struct{ err error }

func (failingWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (f failingWriteCloser) Close() error              { return f.err }

type failingReadCloser struct{ err error }

func (failingReadCloser) Read([]byte) (int, error) { return 0, io.EOF }
func (f failingReadCloser) Close() error           { return f.err }

type trackedReadCloser struct {
	closed bool
	err    error
}

func (*trackedReadCloser) Read([]byte) (int, error) { return 0, io.EOF }

func (c *trackedReadCloser) Close() error {
	c.closed = true
	return c.err
}
