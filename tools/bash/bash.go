package bash

import (
	"context"
	"time"
)

// Executor is the SPI backing [NewTool]. One method, predictable
// shape.
type Executor interface {
	Run(ctx context.Context, in Input) (Output, error)
}

// Input captures everything an executor needs to launch a single
// command. Only Cmd is required.
type Input struct {
	// Cmd is the shell command line. Required.
	Cmd string

	// Timeout bounds the run. 0 = no timeout; ctx cancellation still
	// applies.
	Timeout time.Duration
}

// Output is what every executor returns. A non-zero ExitCode is
// not an error — only spawn/I/O failures populate the error return.
type Output struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Duration time.Duration

	// Killed is true when the process was terminated by ctx or
	// Input.Timeout rather than exiting on its own.
	Killed bool
}
