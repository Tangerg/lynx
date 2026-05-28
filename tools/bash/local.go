package bash

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// defaultMaxOutputBytes caps each captured stream (stdout, stderr).
// 30 KiB mirrors Claude Code's default; large enough for typical
// command output, small enough to keep LLM context bounded even when
// a command misbehaves.
const defaultMaxOutputBytes = 30 * 1024

// LocalExecutor runs commands on the local host via the configured
// shell. Default shell is "/bin/sh -c".
type LocalExecutor struct {
	// Shell is the interpreter; defaults to "/bin/sh".
	Shell string

	// MaxOutputBytes caps the captured size of each stream (stdout
	// and stderr independently). 0 = use [defaultMaxOutputBytes].
	// Bytes beyond the cap are dropped and a "[N bytes truncated]"
	// marker is appended.
	MaxOutputBytes int
}

// NewLocalExecutor returns a [LocalExecutor] with default shell.
func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{Shell: "/bin/sh"}
}

func (l *LocalExecutor) maxOutput() int {
	return cmp.Or(l.MaxOutputBytes, defaultMaxOutputBytes)
}

// Run implements [Executor].
func (l *LocalExecutor) Run(ctx context.Context, in Input) (Output, error) {
	if in.Cmd == "" {
		return Output{}, ErrEmptyCommand
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if in.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, in.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, cmp.Or(l.Shell, "/bin/sh"), "-c", in.Cmd)

	stdout := newBoundedBuffer(l.maxOutput())
	stderr := newBoundedBuffer(l.maxOutput())
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	out := Output{
		Stdout:   stdout.finalize(),
		Stderr:   stderr.finalize(),
		Duration: duration,
	}

	if err != nil {
		exitErr, ok := errors.AsType[*exec.ExitError](err)
		if !ok {
			return out, err
		}
		out.ExitCode = exitErr.ExitCode()
	}

	if runCtx.Err() != nil {
		out.Killed = true
	}
	return out, nil
}

// boundedBuffer is an [io.Writer] that accepts up to `limit` bytes
// and silently drops the rest, counting how many bytes were dropped
// so [boundedBuffer.finalize] can append a truncation marker.
//
// Reporting len(p), nil for writes that are partially or fully
// dropped is deliberate: we don't want to break the child's stdio
// pipe (which would surface as a confusing write error). The
// trade-off is that a runaway command keeps running until the
// command's own timeout / outer ctx fires.
type boundedBuffer struct {
	buf     bytes.Buffer
	limit   int
	dropped int
}

func newBoundedBuffer(limit int) *boundedBuffer {
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	avail := b.limit - b.buf.Len()
	if avail <= 0 {
		b.dropped += len(p)
		return len(p), nil
	}
	if len(p) <= avail {
		return b.buf.Write(p)
	}
	n, _ := b.buf.Write(p[:avail])
	b.dropped += len(p) - n
	return len(p), nil
}

// finalize returns the captured bytes plus a truncation marker if
// anything was dropped. The marker is placed on its own line for
// readability.
func (b *boundedBuffer) finalize() []byte {
	if b.dropped == 0 {
		return b.buf.Bytes()
	}
	out := b.buf.Bytes()
	// Try to cut at the last newline so the marker doesn't dangle
	// mid-line.
	if i := bytes.LastIndexByte(out, '\n'); i > 0 {
		shift := len(out) - (i + 1)
		out = out[:i+1]
		b.dropped += shift
	}
	return fmt.Appendf(out, "... [%d bytes truncated] ...\n", b.dropped)
}
