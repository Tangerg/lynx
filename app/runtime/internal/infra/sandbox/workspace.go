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
	// ErrStopped reports an execution attempted after a successful Stop.
	ErrStopped = errors.New("sandbox: workspace is stopped")
	// ErrShutdown reports access to a backend that has been destroyed.
	ErrShutdown = errors.New("sandbox: workspace is shut down")
	// ErrSnapshotNotFound reports a resume reference absent from durable storage.
	ErrSnapshotNotFound = errors.New("sandbox: snapshot not found")
	// ErrUnavailable reports that no fail-closed command jail exists on the host.
	ErrUnavailable = errors.New("sandbox: command isolation is unavailable on this platform")
)

// SnapshotStore is the durable blob surface a Workspace consumes. Snapshot
// records contain no live process or storage client; Resume receives the store
// again through Config.
type SnapshotStore interface {
	SaveSandboxSnapshot(ctx context.Context, id string, archive []byte) error
	LoadSandboxSnapshot(ctx context.Context, id string) (archive []byte, found bool, err error)
}

// Config supplies process-local resources for a sandbox workspace.
type Config struct {
	// BaseDir owns ephemeral workspace copies and must be a trusted path.
	BaseDir string
	// Store persists content-addressed tar snapshots.
	Store SnapshotStore
	// ReadOnlyPaths re-opens selected host paths hidden by the default policy,
	// typically language toolchains or dependency caches below the user's home.
	ReadOnlyPaths []string
}

type workspaceState uint8

const (
	workspaceRunning workspaceState = iota
	workspaceStopped
	workspaceShutdown
)

type commandRunner interface {
	Run(ctx context.Context, dir string, input toolshell.Input) (toolshell.Output, error)
}

// Workspace is an isolated working copy and a tools/shell Executor. Stop
// captures an immutable snapshot without deleting the backend; Shutdown
// destroys the backend without changing durable snapshots.
//
// It is the isolated-copy model described in the package doc, driven by
// internal/adapter/isolation for a session marked Isolated.
type Workspace struct {
	mu       sync.RWMutex
	dir      string
	store    SnapshotStore
	runner   commandRunner
	state    workspaceState
	snapshot SnapshotID
}

var _ toolshell.Executor = (*Workspace)(nil)

// New creates a fresh isolated working copy. source may be empty; otherwise
// its regular files, directories, and contained relative symlinks are copied
// into the new workspace through the same validated tar format used by Stop.
func New(ctx context.Context, config Config, source string) (*Workspace, error) {
	runner, err := platformRunner(config.ReadOnlyPaths)
	if err != nil {
		return nil, err
	}
	return newWorkspace(ctx, config, source, runner)
}

func newWorkspace(ctx context.Context, config Config, source string, runner commandRunner) (_ *Workspace, err error) {
	if config.Store == nil {
		return nil, errors.New("sandbox: snapshot store is nil")
	}
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
	return &Workspace{dir: dir, store: config.Store, runner: runner}, nil
}

// Resume materializes a new backend from id. The archive digest is verified
// before extraction so corrupted or mismatched durable data never runs.
func Resume(ctx context.Context, config Config, id SnapshotID) (*Workspace, error) {
	runner, err := platformRunner(config.ReadOnlyPaths)
	if err != nil {
		return nil, err
	}
	return resumeWorkspace(ctx, config, id, runner)
}

func resumeWorkspace(ctx context.Context, config Config, id SnapshotID, runner commandRunner) (_ *Workspace, err error) {
	if config.Store == nil {
		return nil, errors.New("sandbox: snapshot store is nil")
	}
	if runner == nil {
		return nil, errors.New("sandbox: command runner is nil")
	}
	if config.BaseDir == "" {
		return nil, errors.New("sandbox: base directory is required")
	}
	if err := id.Validate(); err != nil {
		return nil, err
	}
	archive, found, err := config.Store.LoadSandboxSnapshot(ctx, id.String())
	if err != nil {
		return nil, fmt.Errorf("sandbox: load snapshot %s: %w", id, err)
	}
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotNotFound, id)
	}
	if actual := identifySnapshot(archive); actual != id {
		return nil, fmt.Errorf("sandbox: snapshot %s content digest is %s", id, actual)
	}
	if err := os.MkdirAll(config.BaseDir, 0o700); err != nil {
		return nil, fmt.Errorf("sandbox: create base directory: %w", err)
	}
	dir, err := os.MkdirTemp(config.BaseDir, "workspace-")
	if err != nil {
		return nil, fmt.Errorf("sandbox: create resumed workspace: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()
	if err := extractArchive(ctx, dir, archive); err != nil {
		return nil, fmt.Errorf("sandbox: restore snapshot %s: %w", id, err)
	}
	cleanup = false
	return &Workspace{dir: dir, store: config.Store, runner: runner}, nil
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
// for the command lifetime makes Stop and Shutdown wait for all in-flight work.
func (w *Workspace) Run(ctx context.Context, input toolshell.Input) (toolshell.Output, error) {
	if w == nil {
		return toolshell.Output{}, ErrShutdown
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	switch w.state {
	case workspaceStopped:
		return toolshell.Output{}, ErrStopped
	case workspaceShutdown:
		return toolshell.Output{}, ErrShutdown
	}
	return w.runner.Run(ctx, w.dir, input)
}

// Stop snapshots the workspace and prevents further execution. Repeated calls
// return the first snapshot, making lifecycle retries idempotent.
func (w *Workspace) Stop(ctx context.Context) (SnapshotID, error) {
	if w == nil {
		return "", ErrShutdown
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	switch w.state {
	case workspaceStopped:
		return w.snapshot, nil
	case workspaceShutdown:
		return "", ErrShutdown
	}
	archive, err := archiveTree(ctx, w.dir)
	if err != nil {
		return "", fmt.Errorf("sandbox: snapshot workspace: %w", err)
	}
	id := identifySnapshot(archive)
	if err := w.store.SaveSandboxSnapshot(ctx, id.String(), archive); err != nil {
		return "", fmt.Errorf("sandbox: save snapshot %s: %w", id, err)
	}
	w.snapshot = id
	w.state = workspaceStopped
	return id, nil
}

// Shutdown destroys the process-local backend. Durable snapshots are retained
// for Resume. A failed removal leaves the state retryable.
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
