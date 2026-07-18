// Package lsp drives language servers as child processes over the Language
// Server Protocol, exposing code intelligence (definition, references, hover,
// symbols, diagnostics) to the agent. It is platform-agnostic: the only OS
// dependency is the server binaries the user has installed — the protocol
// itself is plain JSON-RPC over the process's stdio.
//
// Servers is the entry point. It starts one language server per (workspace
// root, language) on first use and keeps it warm for the engine's lifetime.
// Adding a language is a one-line ServerSpec (see server.go); nothing here is
// Go-specific.
package lsp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// ErrNoServer means no configured language server handles the target — an
// unsupported file extension, or a workspace with no matching root marker.
var ErrNoServer = errors.New("lsp: no language server for target")

// ErrClosed means the server set has been shut down.
var ErrClosed = errors.New("lsp: servers closed")

// diagnosticsSettle bounds how long a diagnostics query waits for the server
// to (re)analyze a just-synced document before returning what it has.
const diagnosticsSettle = 3 * time.Second

// Servers owns the live language-server connections for one engine. It is safe
// for concurrent use: many sessions share it, each scoped to its own workspace
// root via the root argument the tools pass.
type Servers struct {
	table  *serverTable
	launch clientLauncher

	mu       sync.Mutex
	clients  map[string]*client      // key: root + "\x00" + server name
	starting map[string]*clientStart // one component-owned launch per key
	closed   bool

	closeOnce sync.Once
	closeErr  error
}

type clientLauncher func(context.Context, ServerSpec, string) (*client, error)

// clientStart is one in-flight first-use launch shared by every concurrent
// caller for the same workspace/server key. The launch belongs to Servers, not
// to the request that happened to arrive first: callers may stop waiting
// independently, while Close cancels and joins the actual launch.
type clientStart struct {
	done       chan struct{}
	cancel     context.CancelFunc
	client     *client
	err        error
	cleanupErr error
}

// NewServers builds a server set over specs (see DefaultServers for the
// built-in table). It starts no processes — servers launch lazily on first use.
func NewServers(specs []ServerSpec) *Servers {
	return &Servers{
		table:    newServerTable(specs),
		launch:   startClient,
		clients:  map[string]*client{},
		starting: map[string]*clientStart{},
	}
}

func clientKey(root, name string) string { return root + "\x00" + name }

// clientForFile resolves the server for abs's extension and returns its
// connection for root, starting it if needed.
func (s *Servers) clientForFile(ctx context.Context, root, abs string) (*client, error) {
	spec, ok := s.table.forFile(abs)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoServer, filepath.Ext(abs))
	}
	return s.clientFor(ctx, root, spec)
}

// clientFor returns the connection for (root, spec), starting it on first use.
// Concurrent callers for the same key share one launch; different keys still
// start independently. The launch is component-owned so cancellation of the
// first request only stops that caller waiting — it cannot tear down a server
// another concurrent caller is waiting for. Close cancels and joins every
// in-flight launch.
func (s *Servers) clientFor(ctx context.Context, root string, spec ServerSpec) (*client, error) {
	key := clientKey(root, spec.Name)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	if c, ok := s.clients[key]; ok {
		s.mu.Unlock()
		return c, nil
	}
	if pending, ok := s.starting[key]; ok {
		s.mu.Unlock()
		return awaitClientStart(ctx, pending)
	}
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return nil, err
	}

	startCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	pending := &clientStart{done: make(chan struct{}), cancel: cancel}
	s.starting[key] = pending
	launch := s.launch
	s.mu.Unlock()

	go s.launchClient(startCtx, key, root, spec, pending, launch)
	return awaitClientStart(ctx, pending)
}

func (s *Servers) launchClient(
	ctx context.Context,
	key, root string,
	spec ServerSpec,
	pending *clientStart,
	launch clientLauncher,
) {
	defer pending.cancel()
	c, err := launch(ctx, spec, root)
	if err == nil && c == nil {
		err = fmt.Errorf("lsp: start %s returned a nil client", spec.Name)
	}
	if err != nil && c != nil {
		err = errors.Join(err, c.close())
		c = nil
	}

	s.mu.Lock()
	delete(s.starting, key)
	if !s.closed {
		pending.client = c
		pending.err = err
		if err == nil {
			s.clients[key] = c
		}
		close(pending.done)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	// Close won the race with a successful handshake. Reclaim the process before
	// publishing completion so Servers.Close really joins every resource it
	// owned, and preserve a cleanup failure in the owner's shutdown result.
	if c != nil {
		pending.cleanupErr = c.close()
	}
	pending.err = ErrClosed
	close(pending.done)
}

func awaitClientStart(ctx context.Context, pending *clientStart) (*client, error) {
	// Prefer a completed launch over a racing caller cancellation.
	select {
	case <-pending.done:
		return pending.client, pending.err
	default:
	}
	select {
	case <-pending.done:
		return pending.client, pending.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// resolve makes file absolute against the workspace root.
func resolve(root, file string) string {
	if filepath.IsAbs(file) {
		return filepath.Clean(file)
	}
	return filepath.Join(root, file)
}

// Definition returns the declaration site(s) of the symbol at pos — 0-based,
// as on the wire. file may be absolute or relative to root.
func (s *Servers) Definition(ctx context.Context, root, file string, pos Position) ([]Location, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.definition(ctx, abs, pos)
}

// References returns the use sites of the symbol at pos, including its
// declaration.
func (s *Servers) References(ctx context.Context, root, file string, pos Position) ([]Location, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.references(ctx, abs, pos)
}

// Implementation returns the concrete implementation site(s) of the interface
// or abstract method at pos.
func (s *Servers) Implementation(ctx context.Context, root, file string, pos Position) ([]Location, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.implementation(ctx, abs, pos)
}

// IncomingCalls returns the callers of the function/method at pos (who calls
// it), as symbols. OutgoingCalls returns its callees (what it calls).
func (s *Servers) IncomingCalls(ctx context.Context, root, file string, pos Position) ([]Symbol, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.callHierarchy(ctx, abs, pos, false)
}

func (s *Servers) OutgoingCalls(ctx context.Context, root, file string, pos Position) ([]Symbol, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.callHierarchy(ctx, abs, pos, true)
}

// Hover returns the server's hover text (signature, doc) at pos, as plain text.
func (s *Servers) Hover(ctx context.Context, root, file string, pos Position) (string, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return "", err
	}
	return c.hover(ctx, abs, pos)
}

// DocumentSymbols returns the symbols declared in file.
func (s *Servers) DocumentSymbols(ctx context.Context, root, file string) ([]Symbol, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.documentSymbols(ctx, abs)
}

// WorkspaceSymbols searches the whole workspace for symbols matching query,
// across every language whose root marker is present. A failed language does
// not discard another language's successful result; when every applicable
// server fails, their errors are joined in configured-server order. Returns
// ErrNoServer when no configured language applies to root.
func (s *Servers) WorkspaceSymbols(ctx context.Context, root, query string) ([]Symbol, error) {
	specs := s.table.forRoot(root)
	if len(specs) == 0 {
		return nil, ErrNoServer
	}
	var out []Symbol
	var errs []error
	succeeded := false
	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c, err := s.clientFor(ctx, root, spec)
		if err != nil {
			errs = append(errs, fmt.Errorf("lsp: start %s for workspace symbols: %w", spec.Name, err))
			continue
		}
		syms, err := c.workspaceSymbols(ctx, query)
		if err != nil {
			errs = append(errs, fmt.Errorf("lsp: query workspace symbols from %s: %w", spec.Name, err))
			continue
		}
		succeeded = true
		out = append(out, syms...)
	}
	if !succeeded {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

// Diagnostics returns the server's current problems for file, syncing it first
// and waiting briefly for a fresh analysis.
func (s *Servers) Diagnostics(ctx context.Context, root, file string) ([]Diagnostic, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.diagnostics(ctx, abs, diagnosticsSettle)
}

// Supported reports whether any configured server handles file's extension —
// lets callers skip LSP work (e.g. post-edit diagnostics) for unsupported
// files without starting anything.
func (s *Servers) Supported(file string) bool {
	_, ok := s.table.forFile(file)
	return ok
}

// Close shuts every live server down. Safe to call multiple times.
func (s *Servers) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		clients := maps.Clone(s.clients)
		starting := maps.Clone(s.starting)
		s.clients = nil
		s.starting = nil
		s.mu.Unlock()

		startKeys := make([]string, 0, len(starting))
		for key := range starting {
			startKeys = append(startKeys, key)
		}
		slices.Sort(startKeys)
		for _, key := range startKeys {
			starting[key].cancel()
		}

		clientKeys := make([]string, 0, len(clients))
		for key := range clients {
			clientKeys = append(clientKeys, key)
		}
		slices.Sort(clientKeys)
		var errs []error
		for _, key := range clientKeys {
			if err := clients[key].close(); err != nil {
				errs = append(errs, err)
			}
		}
		for _, key := range startKeys {
			pending := starting[key]
			<-pending.done
			if pending.cleanupErr != nil {
				errs = append(errs, pending.cleanupErr)
			}
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}
