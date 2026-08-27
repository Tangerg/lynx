package gitprocess

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

const (
	maxOutputBytes = 64 << 20
	maxErrorBytes  = 64 << 10
)

// ErrOutputTooLarge reports a Git command whose stdout cannot enter a Runtime
// VCS read model as one complete value.
var ErrOutputTooLarge = errors.New("gitprocess: output too large")

// Result is one completed Git command. Non-zero process exits are data here so
// the Git use case can interpret documented predicate and diff exit codes.
type Result struct {
	Stdout   []byte
	Stderr   string
	ExitCode int
}

// Run executes Git with bounded stdout and a bounded-drain stderr. overrides
// are command-owned environment entries such as a stable parsing locale.
func Run(ctx context.Context, overrides []string, args ...string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	command := CommandContext(ctx, args...)
	if len(overrides) > 0 {
		command.Env = Environment(overrides...)
	}
	command.WaitDelay = time.Second
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("gitprocess: stdout pipe: %w", err)
	}
	stderr := boundedText{limit: maxErrorBytes}
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return Result{}, fmt.Errorf("gitprocess: start: %w", err)
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxOutputBytes+1))
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		return Result{}, fmt.Errorf("gitprocess: read stdout: %w", readErr)
	}
	if len(output) > maxOutputBytes {
		_ = command.Process.Kill()
		_ = command.Wait()
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		return Result{}, fmt.Errorf("%w: stdout exceeds %d bytes", ErrOutputTooLarge, maxOutputBytes)
	}
	waitErr := command.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	result := Result{Stdout: output, Stderr: stderr.String()}
	if waitErr == nil {
		return result, nil
	}
	if exit, ok := errors.AsType[*exec.ExitError](waitErr); ok {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	return Result{}, fmt.Errorf("gitprocess: wait: %w", waitErr)
}

type boundedText struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedText) Write(value []byte) (int, error) {
	length := len(value)
	remaining := max(b.limit-b.buffer.Len(), 0)
	if len(value) > remaining {
		value = value[:remaining]
		b.truncated = true
	}
	_, _ = b.buffer.Write(value)
	return length, nil
}

func (b *boundedText) String() string {
	if b.truncated {
		return b.buffer.String() + "\n... [git stderr truncated] ..."
	}
	return b.buffer.String()
}
