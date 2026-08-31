package shell

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultShell          = "/bin/sh"
	shellCommandFlag      = "-c"
	defaultMaxOutputBytes = 30 * 1024
	pipeCloseDelay        = time.Second
)

// LocalConfig makes the local process authority visible at construction. The
// directory controls relative-path resolution but is not a filesystem jail;
// callers that need confinement must supply an OS sandbox or container.
type LocalConfig struct {
	Directory      string
	Shell          string
	MaxOutputBytes int
}

// LocalExecutor runs commands on the local host through one immutable
// construction-time configuration.
type LocalExecutor struct {
	directory      string
	shell          string
	maxOutputBytes int
}

func NewLocalExecutor(config LocalConfig) (*LocalExecutor, error) {
	if config.Directory == "" {
		return nil, fmt.Errorf("%w: directory must not be empty", ErrInvalidConfig)
	}
	if strings.TrimSpace(config.Shell) == "" && config.Shell != "" {
		return nil, fmt.Errorf("%w: shell must not be blank", ErrInvalidConfig)
	}
	if config.MaxOutputBytes < 0 {
		return nil, fmt.Errorf("%w: maximum output bytes must not be negative", ErrInvalidConfig)
	}
	directory, err := filepath.Abs(config.Directory)
	if err != nil {
		return nil, fmt.Errorf("shell.NewLocalExecutor: resolve directory %q: %w", config.Directory, err)
	}
	return &LocalExecutor{
		directory:      filepath.Clean(directory),
		shell:          cmp.Or(config.Shell, defaultShell),
		maxOutputBytes: cmp.Or(config.MaxOutputBytes, defaultMaxOutputBytes),
	}, nil
}

func (l *LocalExecutor) Run(ctx context.Context, in Input) (Output, error) {
	if l == nil {
		return Output{}, ErrNilExecutor
	}
	if l.directory == "" || l.shell == "" || l.maxOutputBytes <= 0 {
		return Output{}, ErrInvalidConfig
	}
	if strings.TrimSpace(in.Cmd) == "" {
		return Output{}, ErrEmptyCommand
	}
	if in.Timeout < 0 {
		return Output{}, fmt.Errorf("%w: timeout must not be negative", ErrInvalidInput)
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if in.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, in.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, l.shell, shellCommandFlag, in.Cmd)
	cmd.Dir = l.directory
	// On a timeout/ctx kill, force-close the command's pipes shortly after so
	// Wait returns promptly even when a child the shell spawned still holds them
	// (otherwise Wait blocks until that child exits — the command runs its full
	// duration despite the kill, which is exactly what slow CI runners surface).
	cmd.WaitDelay = pipeCloseDelay

	stdout := newBoundedBuffer(l.maxOutputBytes)
	stderr := newBoundedBuffer(l.maxOutputBytes)
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
// dropped is deliberate: breaking the child's stdio pipe would surface
// as a confusing write error, which is avoided. The
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
