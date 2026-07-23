package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	toolshell "github.com/Tangerg/lynx/tools/shell"
)

var (
	// ErrShutdown reports access to a backend that has been destroyed.
	ErrShutdown = errors.New("sandbox: workspace is shut down")
	// ErrUnavailable reports that no fail-closed command jail exists on the host.
	ErrUnavailable = errors.New("sandbox: command isolation is unavailable on this platform")
)

// Config supplies process-local resources for a sandbox workspace.
type Config struct {
	// BaseDir owns ephemeral workspace copies and must be a trusted path.
	BaseDir string
	// ReadOnlyPaths re-opens selected host paths hidden by the default policy,
	// typically language toolchains or dependency caches below the user's home.
	ReadOnlyPaths []string
}

type workspaceState uint8

const (
	workspaceRunning workspaceState = iota
	workspaceShutdown
)

type commandRunner interface {
	Run(ctx context.Context, dir string, input toolshell.Input) (toolshell.Output, error)
}

// Workspace is an isolated working copy and a tools/shell Executor. Its
// lifetime is process-local: Shutdown removes the scratch copy permanently.
type Workspace struct {
	mu     sync.RWMutex
	dir    string
	runner commandRunner
	state  workspaceState
}

var _ toolshell.Executor = (*Workspace)(nil)

// New creates a fresh isolated working copy. source may be empty; otherwise
// its regular files, directories, and contained relative symlinks are copied
// into the new workspace through a validated tar stream.
func New(ctx context.Context, config Config, source string) (*Workspace, error) {
	runner, err := platformRunner(config.ReadOnlyPaths)
	if err != nil {
		return nil, err
	}
	return newWorkspace(ctx, config, source, runner)
}

func newWorkspace(ctx context.Context, config Config, source string, runner commandRunner) (_ *Workspace, err error) {
	if runner == nil {
		return nil, errors.New("sandbox: command runner is nil")
	}
	if config.BaseDir == "" {
		return nil, errors.New("sandbox: base directory is required")
	}
	if err := os.MkdirAll(config.BaseDir, 0o700); err != nil {
		return nil, fmt.Errorf("sandbox: create base directory: %w", err)
	}
	dir, err := os.MkdirTemp(config.BaseDir, "workspace-")
	if err != nil {
		return nil, fmt.Errorf("sandbox: create workspace: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()
	if source != "" {
		archive, packErr := archiveTree(ctx, source)
		if packErr != nil {
			return nil, fmt.Errorf("sandbox: copy source: %w", packErr)
		}
		if unpackErr := extractArchive(ctx, dir, archive); unpackErr != nil {
			return nil, fmt.Errorf("sandbox: materialize source: %w", unpackErr)
		}
	}
	cleanup = false
	return &Workspace{dir: dir, runner: runner}, nil
}

// Path returns the process-local workspace path while the backend exists.
func (w *Workspace) Path() (string, error) {
	if w == nil {
		return "", ErrShutdown
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.state == workspaceShutdown {
		return "", ErrShutdown
	}
	return w.dir, nil
}

// Run executes one command in the isolated workspace. Holding the read lock
// for the command lifetime makes Shutdown wait for all in-flight work.
func (w *Workspace) Run(ctx context.Context, input toolshell.Input) (toolshell.Output, error) {
	if w == nil {
		return toolshell.Output{}, ErrShutdown
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.state == workspaceShutdown {
		return toolshell.Output{}, ErrShutdown
	}
	return w.runner.Run(ctx, w.dir, input)
}

// Shutdown destroys the process-local backend. A failed removal leaves the
// state retryable.
func (w *Workspace) Shutdown() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state == workspaceShutdown {
		return nil
	}
	if err := os.RemoveAll(w.dir); err != nil {
		return fmt.Errorf("sandbox: remove workspace: %w", err)
	}
	w.state = workspaceShutdown
	return nil
}
