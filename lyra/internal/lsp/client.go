package lsp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

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
	open  map[string]openDoc          // uri → last synced version + content hash
	diags map[string]diagSet          // uri → latest pushed diagnostics
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

// initializeTimeout bounds the initialize handshake. gopls answers initialize
// quickly (indexing continues in the background), but a cold first start on a
// large module can still take a few seconds.
const initializeTimeout = 30 * time.Second

// startClient launches spec's server with its working directory at root,
// wires its stdio to a JSON-RPC connection, and completes the LSP initialize
// handshake. The returned client is ready for queries; the caller owns it and
// must call close.
func startClient(spec ServerSpec, root string) (*client, error) {
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = root
	cmd.Stderr = io.Discard // server logs are noise; failures surface as call errors

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp: start %s: %w", spec.Command, err)
	}

	connCtx, cancel := context.WithCancel(context.Background())
	c := &client{
		spec:    spec,
		root:    root,
		cmd:     cmd,
		cancel:  cancel,
		open:    map[string]openDoc{},
		diags:   map[string]diagSet{},
		updated: make(chan struct{}),
	}
	stream := jsonrpc2.NewBufferedStream(&pipeRWC{out: stdout, in: stdin}, jsonrpc2.VSCodeObjectCodec{})
	c.conn = jsonrpc2.NewConn(connCtx, stream, jsonrpc2.AsyncHandler(c))

	initCtx, initCancel := context.WithTimeout(context.Background(), initializeTimeout)
	defer initCancel()
	if err := c.initialize(initCtx); err != nil {
		_ = c.close()
		return nil, err
	}
	return c, nil
}

func (c *client) initialize(ctx context.Context) error {
	var res json.RawMessage
	params := initializeParams{
		ProcessID:        os.Getpid(),
		RootURI:          pathToURI(c.root),
		Capabilities:     defaultCapabilities(),
		WorkspaceFolders: []workspaceFolder{{URI: pathToURI(c.root), Name: filepath.Base(c.root)}},
	}
	if err := c.conn.Call(ctx, "initialize", params, &res); err != nil {
		return fmt.Errorf("lsp: initialize %s: %w", c.spec.Name, err)
	}
	if err := c.conn.Notify(ctx, "initialized", struct{}{}); err != nil {
		return fmt.Errorf("lsp: initialized %s: %w", c.spec.Name, err)
	}
	return nil
}

// Handle implements jsonrpc2.Handler — the server→client direction. We cache
// diagnostics pushes and answer the handful of requests gopls makes during
// startup; everything else is acknowledged (requests) or ignored
// (notifications) so the server is never left blocking on us.
func (c *client) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	switch req.Method {
	case "textDocument/publishDiagnostics":
		var p publishDiagnosticsParams
		if req.Params != nil {
			_ = json.Unmarshal(*req.Params, &p)
		}
		c.storeDiagnostics(p)
	case "workspace/configuration":
		// Reply one null per requested item — we expose no settings, and gopls
		// treats null as "use defaults".
		n := 1
		if req.Params != nil {
			var cp struct {
				Items []json.RawMessage `json:"items"`
			}
			if json.Unmarshal(*req.Params, &cp) == nil {
				n = len(cp.Items)
			}
		}
		_ = conn.Reply(ctx, req.ID, make([]*struct{}, n))
	default:
		// Acknowledge any other server request (registerCapability,
		// workDoneProgress/create, …) with null so it isn't left waiting;
		// ignore unknown notifications.
		if !req.Notif {
			_ = conn.Reply(ctx, req.ID, nil)
		}
	}
}

func (c *client) storeDiagnostics(p publishDiagnosticsParams) {
	c.mu.Lock()
	c.diags[p.URI] = diagSet{version: p.Version, diagnostics: p.Diagnostics}
	close(c.updated)
	c.updated = make(chan struct{})
	c.mu.Unlock()
}

// ensureOpen makes the server aware of abs's current on-disk content: a
// didOpen the first time, a didChange when the content has changed since we
// last synced (the agent edits files out-of-band). It returns the document
// version now in effect, which a diagnostics wait uses to recognize fresh
// pushes. A no-op (content unchanged) returns the existing version.
func (c *client) ensureOpen(ctx context.Context, abs string) (int, error) {
	text, err := os.ReadFile(abs)
	if err != nil {
		return 0, fmt.Errorf("lsp: read %s: %w", abs, err)
	}
	uri := pathToURI(abs)
	hash := sha256.Sum256(text)

	c.mu.Lock()
	prev, isOpen := c.open[uri]
	if isOpen && prev.hash == hash {
		c.mu.Unlock()
		return prev.version, nil
	}
	version := prev.version + 1
	c.open[uri] = openDoc{version: version, hash: hash}
	c.mu.Unlock()

	if !isOpen {
		err = c.conn.Notify(ctx, "textDocument/didOpen", didOpenParams{
			TextDocument: textDocumentItem{URI: uri, LanguageID: c.spec.LanguageID, Version: version, Text: string(text)},
		})
	} else {
		err = c.conn.Notify(ctx, "textDocument/didChange", didChangeParams{
			TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: version},
			ContentChanges: []contentChange{{Text: string(text)}},
		})
	}
	if err != nil {
		return 0, fmt.Errorf("lsp: sync %s: %w", abs, err)
	}
	return version, nil
}

func (c *client) definition(ctx context.Context, abs string, pos Position) ([]Location, error) {
	if _, err := c.ensureOpen(ctx, abs); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := c.conn.Call(ctx, "textDocument/definition", positionParams{
		TextDocument: textDocumentIdentifier{URI: pathToURI(abs)},
		Position:     pos,
	}, &raw); err != nil {
		return nil, fmt.Errorf("lsp: definition: %w", err)
	}
	return parseLocations(raw), nil
}

func (c *client) references(ctx context.Context, abs string, pos Position) ([]Location, error) {
	if _, err := c.ensureOpen(ctx, abs); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := c.conn.Call(ctx, "textDocument/references", referenceParams{
		TextDocument: textDocumentIdentifier{URI: pathToURI(abs)},
		Position:     pos,
		Context:      referenceContext{IncludeDeclaration: true},
	}, &raw); err != nil {
		return nil, fmt.Errorf("lsp: references: %w", err)
	}
	return parseLocations(raw), nil
}

func (c *client) hover(ctx context.Context, abs string, pos Position) (string, error) {
	if _, err := c.ensureOpen(ctx, abs); err != nil {
		return "", err
	}
	var h struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := c.conn.Call(ctx, "textDocument/hover", positionParams{
		TextDocument: textDocumentIdentifier{URI: pathToURI(abs)},
		Position:     pos,
	}, &h); err != nil {
		return "", fmt.Errorf("lsp: hover: %w", err)
	}
	return hoverText(h.Contents), nil
}

func (c *client) documentSymbols(ctx context.Context, abs string) ([]Symbol, error) {
	if _, err := c.ensureOpen(ctx, abs); err != nil {
		return nil, err
	}
	uri := pathToURI(abs)
	var raw json.RawMessage
	if err := c.conn.Call(ctx, "textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	}, &raw); err != nil {
		return nil, fmt.Errorf("lsp: documentSymbol: %w", err)
	}
	return parseSymbols(raw, uri), nil
}

func (c *client) workspaceSymbols(ctx context.Context, query string) ([]Symbol, error) {
	var infos []symbolInformation
	if err := c.conn.Call(ctx, "workspace/symbol", workspaceSymbolParams{Query: query}, &infos); err != nil {
		return nil, fmt.Errorf("lsp: workspace/symbol: %w", err)
	}
	out := make([]Symbol, 0, len(infos))
	for _, s := range infos {
		out = append(out, Symbol{Name: s.Name, Kind: s.Kind, Location: s.Location, Container: s.ContainerName})
	}
	return out, nil
}

// diagnostics syncs abs and returns the server's diagnostics for it. Servers
// push diagnostics asynchronously after a change, so we wait up to settle for
// a push at least as fresh as the version we just synced before returning the
// cache (which may already hold a fresh-enough set, ending the wait at once).
func (c *client) diagnostics(ctx context.Context, abs string, settle time.Duration) ([]Diagnostic, error) {
	version, err := c.ensureOpen(ctx, abs)
	if err != nil {
		return nil, err
	}
	uri := pathToURI(abs)
	deadline := time.NewTimer(settle)
	defer deadline.Stop()
	for {
		c.mu.Lock()
		ds, ok := c.diags[uri]
		wait := c.updated
		c.mu.Unlock()
		if ok && ds.version >= version {
			return ds.diagnostics, nil
		}
		select {
		case <-wait: // a push arrived — re-check
		case <-deadline.C:
			c.mu.Lock()
			ds := c.diags[uri]
			c.mu.Unlock()
			return ds.diagnostics, nil // best effort: whatever we have
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// close shuts the server down: a best-effort graceful shutdown/exit, then the
// connection (which closes stdin), then a hard process kill as a backstop.
// Safe to call more than once.
func (c *client) close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.conn.Call(ctx, "shutdown", nil, nil)
	_ = c.conn.Notify(ctx, "exit", nil)
	c.cancel()
	_ = c.conn.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	return nil
}

// pipeRWC adapts a child process's separate stdout (read) and stdin (write)
// pipes into the single io.ReadWriteCloser the JSON-RPC stream expects.
type pipeRWC struct {
	out io.ReadCloser
	in  io.WriteCloser
}

func (p *pipeRWC) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *pipeRWC) Write(b []byte) (int, error) { return p.in.Write(b) }

func (p *pipeRWC) Close() error {
	werr := p.in.Close()
	rerr := p.out.Close()
	if werr != nil {
		return werr
	}
	return rerr
}
