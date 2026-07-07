package lsp

import (
	"context"
	"os/exec"
	"sync"

	"github.com/sourcegraph/jsonrpc2"
)

// client is one live connection to one language-server process, scoped to one
// workspace root. It owns the child process, the JSON-RPC connection over its
// stdio, the set of documents it has opened, and a cache of the diagnostics
// the server has pushed. It implements jsonrpc2.Handler to receive the
// server→client traffic (diagnostics pushes, configuration requests).
type client struct {
	spec ServerSpec
	root string

	cmd    *exec.Cmd
	conn   *jsonrpc2.Conn
	cancel context.CancelFunc // tears down the connection's read loop

	mu    sync.Mutex
	open  map[string]openDoc // uri → last synced version + content hash
	diags map[string]diagSet // uri → latest pushed diagnostics
	// updated is closed (and replaced) on every diagnostics push, so a waiter
	// can block for the next one without polling.
	updated chan struct{}
}

type openDoc struct {
	version int
	hash    [32]byte
}

type diagSet struct {
	version     int
	diagnostics []Diagnostic
}
