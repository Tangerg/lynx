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
	"path/filepath"
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
	table *serverTable

	mu      sync.Mutex
	clients map[string]*client // key: root + "\x00" + server name
	closed  bool
}

// NewServers builds a server set over specs (see DefaultServers for the
// built-in table). It starts no processes — servers launch lazily on first use.
func NewServers(specs []ServerSpec) *Servers {
	return &Servers{table: newServerTable(specs), clients: map[string]*client{}}
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
// The handshake runs outside the lock so a slow server start doesn't stall
// other languages' queries; a concurrent loser of the start race closes its
// surplus client and returns the winner. ctx scopes the (first-use) handshake
// and carries the trace span onto the launched connection.
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
	s.mu.Unlock()

	c, err := startClient(ctx, spec, root)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		_ = c.close()
		return nil, ErrClosed
	}
	if existing, ok := s.clients[key]; ok {
		_ = c.close() // lost the race — keep the established one
		return existing, nil
	}
	s.clients[key] = c
	return c, nil
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
// across every language whose root marker is present. Returns ErrNoServer when
// no configured language applies to root.
func (s *Servers) WorkspaceSymbols(ctx context.Context, root, query string) ([]Symbol, error) {
	specs := s.table.forRoot(root)
	if len(specs) == 0 {
		return nil, ErrNoServer
	}
	var out []Symbol
	for _, spec := range specs {
		c, err := s.clientFor(ctx, root, spec)
		if err != nil {
			continue // one language's server failing shouldn't sink the rest
		}
		syms, err := c.workspaceSymbols(ctx, query)
		if err != nil {
			continue
		}
		out = append(out, syms...)
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
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	clients := s.clients
	s.clients = nil
	s.mu.Unlock()

	var errs []error
	for _, c := range clients {
		if err := c.close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
