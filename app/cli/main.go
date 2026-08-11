// Command lyra is the terminal front end for the lyra agent runtime.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/cmd"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeembedded"
)

const mockRuntimeNotice = "lyra: scripted mock runtime (explicit test/demo mode)"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := processSignalContext(context.Background())
	defer stop()

	owner, notice, err := newRuntimeOwner()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
		return exitCode(err)
	}
	root := cmd.NewRoot(cmd.Dependencies{
		OpenRuntime:   owner.Runtime,
		RuntimeNotice: notice,
	})
	root.SetIn(os.Stdin)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	err = root.ExecuteContext(ctx)
	err = errors.Join(err, owner.Close())
	if cause := context.Cause(ctx); cause != nil {
		err = errors.Join(cause, err)
	}
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
	return exitCode(err)
}

type runtimeOwner interface {
	Runtime(context.Context) (agent.Runtime, error)
	Close() error
}

type mockOwner struct{ runtime agent.Runtime }

func (o *mockOwner) Runtime(context.Context) (agent.Runtime, error) { return o.runtime, nil }
func (*mockOwner) Close() error                                     { return nil }

func newRuntimeOwner() (runtimeOwner, string, error) {
	switch mode := os.Getenv("LYRA_RUNTIME"); mode {
	case "mock":
		return &mockOwner{runtime: mock.New()}, mockRuntimeNotice, nil
	case "", "embedded":
	default:
		return nil, "", fmt.Errorf("unsupported LYRA_RUNTIME %q (want embedded or mock)", mode)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("resolve runtime home: %w", err)
	}
	lyraHome := os.Getenv("LYRA_HOME")
	if lyraHome == "" {
		lyraHome = filepath.Join(userHome, ".lyra")
	}
	if !filepath.IsAbs(lyraHome) {
		return nil, "", errors.New("LYRA_HOME must be an absolute path")
	}
	runtimeDirectory := filepath.Join(filepath.Clean(lyraHome), "runtime")
	configDirectories, err := runtimeConfigDirectories(runtimeDirectory)
	if err != nil {
		return nil, "", err
	}
	return runtimeembedded.NewOwner(runtimeembedded.Config{
		DataDirectory: runtimeDirectory, UserHomePath: userHome,
		ConfigDirectories: configDirectories, ClientVersion: cmd.Version(),
	}), "", nil
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
