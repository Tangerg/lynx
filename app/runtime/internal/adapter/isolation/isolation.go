// Package isolation owns the per-session sandbox working copies that back an
// isolated run: a [session.Session] marked Isolated runs its tools inside a
// throwaway tar-copy of its project directory (the sandbox Workspace) rather
// than the real tree, so file changes never touch the project and the jailed
// shell cannot reach the network. It is the adapter that activates the C7
// isolated-copy sandbox — wrapping infra/sandbox behind the narrow ports the
// runs coordinator (resolve) and the session-delete cascade (discard) consume.
package isolation

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/sandbox"
)

// Isolator holds one sandbox Workspace per isolated session. The copy is
// created lazily on the session's first isolated run and reused across its
// turns (so the agent's work accumulates), then destroyed when the session is
// deleted or the runtime stops. The copy is a scratch tree: it is never
// snapshotted or written back — isolation means the project is left untouched.
// Safe for concurrent use.
type Isolator struct {
	baseDir       string
	readOnlyPaths []string

	mu         sync.Mutex
	workspaces map[string]*sandbox.Workspace
}

// New builds an isolator rooting its ephemeral copies under baseDir (a trusted
// path owned by the runtime). readOnlyPaths re-opens toolchain roots below the
// hidden home for the jailed shell.
func New(baseDir string, readOnlyPaths []string) *Isolator {
	return &Isolator{
		baseDir:       baseDir,
		readOnlyPaths: readOnlyPaths,
		workspaces:    map[string]*sandbox.Workspace{},
	}
}

// Workspace returns the isolated working-copy directory for sessionID, creating
// it from projectRoot (a tar copy of the real project) on first use and reusing
// it thereafter. It fails closed with [sandbox.ErrUnavailable] when the host has
// no isolation backend, so an isolated run is refused rather than run
// unconfined.
func (i *Isolator) Workspace(ctx context.Context, sessionID, projectRoot string) (string, error) {
	if ws := i.lookup(sessionID); ws != nil {
		return ws.Path()
	}
	// Materialize the copy OUTSIDE the lock: a tar copy is slow I/O and must not
	// block another session's Workspace/Discard. A session's runs are serialized
	// by admission, so the same session cannot race here; the store-back below
	// still resolves a race defensively (discarding the losing copy).
	fresh, err := sandbox.New(ctx, sandbox.Config{
		BaseDir:       i.baseDir,
		ReadOnlyPaths: i.readOnlyPaths,
	}, projectRoot)
	if err != nil {
		return "", err
	}
	i.mu.Lock()
	if existing := i.workspaces[sessionID]; existing != nil {
		i.mu.Unlock()
		_ = fresh.Shutdown()
		return existing.Path()
	}
	i.workspaces[sessionID] = fresh
	i.mu.Unlock()
	return fresh.Path()
}

func (i *Isolator) lookup(sessionID string) *sandbox.Workspace {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.workspaces[sessionID]
}

// Discard destroys a session's isolated working copy. Idempotent: a session
// that never ran isolated (no workspace) is a no-op. Called from the
// session-delete cascade — deleting a session takes its scratch copy with it.
func (i *Isolator) Discard(sessionID string) error {
	i.mu.Lock()
	ws := i.workspaces[sessionID]
	delete(i.workspaces, sessionID)
	i.mu.Unlock()
	if ws == nil {
		return nil
	}
	return ws.Shutdown()
}

// Close destroys every live working copy — the process-shutdown closer. It
// joins every teardown error.
func (i *Isolator) Close() error {
	i.mu.Lock()
	workspaces := i.workspaces
	i.workspaces = map[string]*sandbox.Workspace{}
	i.mu.Unlock()
	var errs []error
	for _, ws := range workspaces {
		if err := ws.Shutdown(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
