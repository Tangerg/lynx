// Command lyra is the terminal front end for the lyra agent runtime.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/client/mock"
	"github.com/Tangerg/lynx/app/cli/internal/cmd"
)

const mockRuntimeNotice = "lyra: scripted mock runtime — no backend is wired in yet"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := processSignalContext(context.Background())
	defer stop()

	root := cmd.NewRoot(cmd.Dependencies{
		OpenRuntime: func(context.Context) (client.Runtime, error) {
			return mock.New(), nil
		},
		RuntimeNotice: mockRuntimeNotice,
	})
	root.SetIn(os.Stdin)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	err := root.ExecuteContext(ctx)
	if cause := context.Cause(ctx); cause != nil {
		return exitCode(cause)
	}
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
	return exitCode(err)
}

type exitCoder interface {
	error
	ExitCode() int
}

func exitCode(err error) int {
	if coded, ok := errors.AsType[exitCoder](err); ok {
		return coded.ExitCode()
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return 1
}

type processSignalError struct{ signal os.Signal }

func (e processSignalError) Error() string { return fmt.Sprintf("terminated by %s", e.signal) }

func (e processSignalError) Unwrap() error { return context.Canceled }

func (e processSignalError) ExitCode() int {
	switch e.signal {
	case os.Interrupt:
		return 130
	case syscall.SIGTERM:
		return 143
	default:
		return 1
	}
}

func processSignalContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case received := <-signals:
			// Restore the default handler after the first request so a second signal
			// can still terminate a process whose graceful shutdown is stuck.
			signal.Stop(signals)
			cancel(processSignalError{signal: received})
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(signals)
		cancel(context.Canceled)
		<-done
	}
}
