// Package isolation owns the per-session sandbox working copies that back an
// isolated run: a [session.Session] marked Isolated runs its tools inside a
// throwaway tar-copy of its project directory (the sandbox Workspace) rather
// than the real tree, so file changes never touch the project and the jailed
// shell cannot reach the network. It is the adapter that activates the C7
// isolated-copy sandbox — wrapping infra/sandbox behind the narrow ports the
// runs coordinator (create/resolve) and the session-delete cascade (discard)
// consume.
package isolation

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/sandbox"
)

// Isolator holds one sandbox Workspace per isolated session. The copy is
// created lazily on the session's first isolated run and reused across its
// turns (so the agent's work accumulates), then snapshotted and destroyed when
// the session is deleted. Safe for concurrent use.
type Isolator struct {
	baseDir       string
	store         sandbox.SnapshotStore
	readOnlyPaths []string

	mu         sync.Mutex
	workspaces map[string]*sandbox.Workspace
}

// New builds an isolator rooting its ephemeral copies under baseDir (a trusted
// path owned by the runtime). store persists the final snapshot taken when a
// session is discarded; readOnlyPaths re-opens toolchain roots below the hidden
// home for the jailed shell.
func New(baseDir string, store sandbox.SnapshotStore, readOnlyPaths []string) *Isolator {
	return &Isolator{
		baseDir:       baseDir,
		store:         store,
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
	i.mu.Lock()
	defer i.mu.Unlock()
	if ws, ok := i.workspaces[sessionID]; ok {
		return ws.Path()
	}
	ws, err := sandbox.New(ctx, sandbox.Config{
		BaseDir:       i.baseDir,
		Store:         i.store,
		ReadOnlyPaths: i.readOnlyPaths,
	}, projectRoot)
	if err != nil {
		return "", err
	}
	path, err := ws.Path()
	if err != nil {
		return "", err
	}
	i.workspaces[sessionID] = ws
	return path, nil
}

// Discard snapshots the session's final isolated state (best-effort) and
// destroys its working copy. Idempotent: a session that never ran isolated (no
// workspace) is a no-op. Called from the session-delete cascade.
func (i *Isolator) Discard(ctx context.Context, sessionID string) error {
	i.mu.Lock()
	ws := i.workspaces[sessionID]
	delete(i.workspaces, sessionID)
	i.mu.Unlock()
	if ws == nil {
		return nil
	}
	// The final snapshot is a forensic convenience, not a durability guarantee;
	// a snapshot failure must not block destroying the copy.
	_, _ = ws.Stop(ctx)
	return ws.Shutdown()
}

// Close destroys every live working copy — the process-shutdown closer. It
// skips the final snapshot (shutdown is not a forensic checkpoint) and joins
// every teardown error.
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
