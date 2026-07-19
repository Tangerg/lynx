package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

// initializeTimeout bounds the initialize handshake. gopls answers initialize
// quickly (indexing continues in the background), but a cold first start on a
// large module can still take a few seconds.
const initializeTimeout = 30 * time.Second

// clientShutdownTimeout bounds the protocol-level shutdown handshake. Once it
// expires, close hard-stops and reaps the owned child process.
const clientShutdownTimeout = 2 * time.Second

// startClient launches spec's server with its working directory at root,
// wires its stdio to a JSON-RPC connection, and completes the LSP initialize
// handshake. The returned client is ready for queries; the caller owns it and
// must call close. ctx scopes only the synchronous handshake; the connection's
// own read loop is detached from it (it must outlive the call that started it —
// the server stays warm for the engine's lifetime) while keeping ctx's trace
// span via context.WithoutCancel.
func startClient(ctx context.Context, spec ServerSpec, root string) (*client, error) {
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = root
	cmd.Stderr = io.Discard // server logs are noise; failures surface as call errors

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdout pipe: %w", err)
	}
	pipes := &pipeRWC{out: stdout}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, closeUnstartedPipes(spec.Command, pipes, fmt.Errorf("lsp: stdin pipe: %w", err))
	}
	pipes.in = stdin
	if err := cmd.Start(); err != nil {
		return nil, closeUnstartedPipes(spec.Command, pipes, fmt.Errorf("lsp: start %s: %w", spec.Command, err))
	}
	wait := make(chan error, 1)
	go func() {
		wait <- cmd.Wait()
		close(wait)
	}()

	// WithoutCancel: the read loop outlives this call (the connection is cached
	// and reused) so it must not die when ctx ends, but keeping ctx's values
	// preserves the trace span instead of severing it with context.Background().
	connCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c := &client{
		spec:    spec,
		root:    root,
		cmd:     cmd,
		cancel:  cancel,
		wait:    wait,
		open:    map[string]openDoc{},
		diags:   map[string]diagSet{},
		updated: make(chan struct{}),
	}
	stream := jsonrpc2.NewBufferedStream(pipes, jsonrpc2.VSCodeObjectCodec{})
	c.conn = jsonrpc2.NewConn(connCtx, stream, jsonrpc2.AsyncHandler(c))

	// The handshake is synchronous within this call, so it rides ctx directly —
	// keeping the trace span and honoring caller cancellation — bounded by the
	// initialize timeout.
	initCtx, initCancel := context.WithTimeout(ctx, initializeTimeout)
	defer initCancel()
	if err := c.initialize(initCtx); err != nil {
		if cleanupErr := c.close(); cleanupErr != nil {
			return nil, errors.Join(err, cleanupErr)
		}
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

// close shuts the server down: a best-effort graceful shutdown/exit, then the
// connection (which closes stdin), then a hard process kill as a backstop.
// Safe to call more than once.
func (c *client) close() error {
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), clientShutdownTimeout)
		defer cancel()

		// Protocol shutdown is advisory: a crashed or wedged server still needs
		// its local resources reclaimed below.
		_ = c.conn.Call(ctx, "shutdown", nil, nil)
		_ = c.conn.Notify(ctx, "exit", nil)
		c.cancel()

		var errs []error
		if err := c.conn.Close(); err != nil && !errors.Is(err, jsonrpc2.ErrClosed) {
			errs = append(errs, fmt.Errorf("lsp: close %s connection: %w", c.spec.Name, err))
		}

		select {
		case err := <-c.wait:
			if err != nil {
				errs = append(errs, fmt.Errorf("lsp: wait for %s shutdown: %w", c.spec.Name, err))
			}
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf(
				"lsp: graceful shutdown of %s exceeded %s: %w",
				c.spec.Name,
				clientShutdownTimeout,
				ctx.Err(),
			))
			if err := killAndJoinProcess(c.spec.Name, c.cmd.Process, c.wait); err != nil {
				errs = append(errs, err)
			}
		}
		c.closeErr = errors.Join(errs...)
	})
	return c.closeErr
}

// killAndJoinProcess is the hard-stop half of client ownership. cmd.Wait has a
// dedicated goroutine, so this function sends the terminal signal and consumes
// that goroutine's result before returning. The resulting ExitError is expected
// after Kill; the caller already reports why a hard stop was necessary.
func killAndJoinProcess(name string, process *os.Process, wait <-chan error) error {
	if process == nil {
		return fmt.Errorf("lsp: kill unresponsive %s: process is unavailable", name)
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("lsp: kill unresponsive %s: %w", name, err)
	}
	<-wait
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
	return errors.Join(closePipe(p.in), closePipe(p.out))
}

func closePipe(pipe io.Closer) error {
	if pipe == nil {
		return nil
	}
	return pipe.Close()
}

func closeUnstartedPipes(command string, pipes *pipeRWC, cause error) error {
	if err := pipes.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("lsp: close %s pipes after launch failure: %w", command, err))
	}
	return cause
}
