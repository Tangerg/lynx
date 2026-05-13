// Package bash exposes a single LLM-callable shell tool plus a small
// [Executor] SPI. The package itself does not pick where commands run
// — local, sandboxed, or remote backends each implement [Executor] and
// plug in via [NewTool].
//
// The local executor ([NewLocalExecutor]) is the reference impl and
// covers the common case (run on the same host as the agent).
package bash

import (
	"context"
	"time"
)

// Executor is the SPI backing [NewTool]. One method, predictable
// shape.
type Executor interface {
	Run(ctx context.Context, in RunInput) (RunOutput, error)
}

// RunInput captures everything an executor needs to launch a single
// command. Only Cmd is required.
type RunInput struct {
	// Cmd is the shell command line. Required.
	Cmd string

	// Timeout bounds the run. 0 = no timeout; ctx cancellation still
	// applies.
	Timeout time.Duration
}

// RunOutput is what every executor returns. A non-zero ExitCode is
// not an error — only spawn/I/O failures populate the error return.
type RunOutput struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Duration time.Duration

	// Killed is true when the process was terminated by ctx or
	// RunInput.Timeout rather than exiting on its own.
	Killed bool
}
