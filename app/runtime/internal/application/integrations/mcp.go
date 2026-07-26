package integrations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/httporigin"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

const mcpReconcileTimeout = 30 * time.Second

// errClosed reports that a post-commit reconcile / background task could not be
// launched because the component is shutting down.
var errClosed = errors.New("integrations: closed")

// errMCPConnectionUnavailable reports an incomplete coordinator assembly at the
// use-case boundary instead of letting a detached task fail asynchronously.
var errMCPConnectionUnavailable = errors.New("integrations: MCP connection service is unavailable")

// ErrInvalidMCPServerConfiguration marks a malformed MCP configuration command.
// Outer adapters map it to their validation error without re-running domain
// validation or inspecting persistence state.
var ErrInvalidMCPServerConfiguration = errors.New("integrations: invalid MCP server configuration")

// ErrUnknownMCPServer is the application boundary's stable unknown-server
// result. The underlying domain sentinel remains internal to this package.
var ErrUnknownMCPServer = errors.New("integrations: unknown MCP server")

// ErrMCPServerDisabled reports a connection command against a configured
// server whose durable enablement gate is closed.
var ErrMCPServerDisabled = errors.New("integrations: MCP server is disabled")

// resolveMCPServerConfiguration applies the registry-owned credential policy to
// an editable command. An omitted HTTP Authorization retains a stored token only
// when the transport and origin remain unchanged; credentials never silently
// cross to a different endpoint.
func (c *Coordinator) resolveMCPServerConfiguration(ctx context.Context, candidate mcpserver.Server) (mcpserver.Server, error) {
	if candidate.Authorization != "" || candidate.Name == "" || c.mcpRegistry == nil {
		return candidate, nil
	}
	current, found, err := c.mcpRegistry.Get(ctx, candidate.Name)
	if err != nil || !found {
		return candidate, err
	}
	if current.Transport == mcpserver.TransportStreamableHTTP &&
		candidate.Transport == mcpserver.TransportStreamableHTTP &&
		httporigin.Same(current.URL, candidate.URL) {
		candidate.Authorization = current.Authorization
	}
	return candidate, nil
}

// ConfigureMCPServer upserts a server in the registry and applies it to the live
// connections: an enabled server is (re)dialed, a disabled one is dropped from
// the live set (it stays in the registry). A dial failure does not fail the call
// — the server is persisted and tracked "failed" (reconnectable); the
// connectivity feedback path is TestMCPServer.
func (c *Coordinator) ConfigureMCPServer(ctx context.Context, input MCPServerInput) (MCPServerConfig, error) {
	candidate, err := mcpServerCandidate(input)
	if err != nil {
		return MCPServerConfig{}, err
	}
	write, err := c.beginMCPWrite(ctx)
	if err != nil {
		return MCPServerConfig{}, err
	}
	defer write.close()
	// Credential retention reads the current durable entry inside the same
	// mutation order as the following upsert. Resolving it before admission would
	// let a concurrent configure overwrite a newer token with a stale snapshot.
	srv, err := c.resolveMCPServerConfiguration(write.requestCtx, candidate)
	if err != nil {
		return MCPServerConfig{}, err
	}
	if err := c.mcpRegistry.Configure(write.requestCtx, srv); err != nil {
		return MCPServerConfig{}, err
	}
	reconcileCtx, cancel := context.WithTimeout(write.ownerCtx, mcpReconcileTimeout)
	defer cancel()
	if err := c.applyMCPRegistryChange(reconcileCtx, srv); err != nil {
		return MCPServerConfig{}, err
	}
	var statusEvent mcpStatusEvent
	if !srv.Enabled {
		statusEvent = c.prepareMCPStatus(MCPServerStatus{Name: srv.Name})
	}
	write.unlock()
	if srv.Enabled {
		c.redialMCPServer(write.ownerCtx, srv)
	} else {
		c.publishMCPStatus(statusEvent)
	}
	return mcpConfigView(srv), nil
}

// RemoveMCPServer deletes a server from the registry and drops it from the live
// connections.
func (c *Coordinator) RemoveMCPServer(ctx context.Context, name string) error {
	write, err := c.beginMCPWrite(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	c.cancelMCPDial(name)
	if err := c.mcpRegistry.Remove(write.requestCtx, name); err != nil {
		return err
	}
	reconcileCtx, cancel := context.WithTimeout(write.ownerCtx, mcpReconcileTimeout)
	defer cancel()
	// Shrink the live set before publishing the new policy: dropping tools can't
	// expose a hidden one, but publishing first would leave the about-to-be-dropped
	// tools briefly live under the wrong policy.
	var projectionErr error
	if c.mcpRegistryCommands != nil {
		projectionErr = c.mcpRegistryCommands.Detach(name)
	}
	policyErr := c.refreshMCPToolPolicy(reconcileCtx)
	var statusEvent mcpStatusEvent
	if policyErr == nil {
		statusEvent = c.prepareMCPStatus(MCPServerStatus{Name: name})
	}
	write.unlock()
	c.publishMCPStatus(statusEvent)
	return errors.Join(projectionErr, policyErr)
}

// SetMCPServerEnabled flips a server's enablement in the registry and applies it
// to the live connections (enable → dial, disable → drop).
func (c *Coordinator) SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error {
	write, err := c.beginMCPWrite(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	if err := c.mcpRegistry.SetEnabled(write.requestCtx, name, enabled); err != nil {
		return err
	}
	reconcileCtx, cancel := context.WithTimeout(write.ownerCtx, mcpReconcileTimeout)
	defer cancel()
	srv, ok, err := c.mcpRegistry.Get(reconcileCtx, name)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUnknownMCPServer
	}
	if !srv.Enabled {
		c.cancelMCPDial(name)
	}
	if err := c.applyMCPRegistryChange(reconcileCtx, srv); err != nil {
		return err
	}
	var statusEvent mcpStatusEvent
	if !srv.Enabled {
		statusEvent = c.prepareMCPStatus(MCPServerStatus{Name: srv.Name})
	}
	write.unlock()
	if srv.Enabled {
		c.redialMCPServer(write.ownerCtx, srv)
	} else {
		c.publishMCPStatus(statusEvent)
	}
	return nil
}

type mcpWrite struct {
	coordinator *Coordinator
	requestCtx  context.Context
	ownerCtx    context.Context
	finish      func()
	locked      bool
}

// beginMCPWrite owns both task scopes and the durable mutation lock. Callers
// release the lock before invoking status sinks or dispatching live dials, then
// defer close to release the task scopes on every exit.
func (c *Coordinator) beginMCPWrite(ctx context.Context) (*mcpWrite, error) {
	ownerCtx, releaseOwner, ok := c.tasks.Attach(ctx)
	if !ok {
		return nil, errClosed
	}
	requestCtx, releaseRequest, ok := c.tasks.AttachLinked(ctx)
	if !ok {
		releaseOwner()
		return nil, errClosed
	}
	c.mcpMutationMu.Lock()
	if err := requestCtx.Err(); err != nil {
		c.mcpMutationMu.Unlock()
		releaseRequest()
		releaseOwner()
		return nil, err
	}
	return &mcpWrite{
		coordinator: c,
		requestCtx:  requestCtx,
		ownerCtx:    ownerCtx,
		finish: func() {
			releaseRequest()
			releaseOwner()
		},
		locked: true,
	}, nil
}

func (write *mcpWrite) unlock() {
	if write != nil && write.locked {
		write.locked = false
		write.coordinator.mcpMutationMu.Unlock()
	}
}

func (write *mcpWrite) close() {
	if write == nil || write.finish == nil {
		return
	}
	write.unlock()
	write.finish()
	write.finish = nil
}

// applyMCPRegistryChange reflects a persisted registry entry into the policy
// snapshot and, when disabling, the live tool set — all under the caller's
// mutation lock. Publication order keeps disabled tools from becoming momentarily
// visible:
//   - enabling publishes policy here; the live (re)dial is NOT done here — the
//     caller dispatches it detached, after releasing the lock, because a network
//     handshake must never hold the control-plane lock (see ConfigureMCPServer);
//   - disabling detaches the live projection before publishing policy; physical
//     session retirement remains owned by the adapter's shutdown lifecycle.
//
// Either reversal would leave a window where a disabled tool is live under the
// wrong policy. The caller has already mutated the registry, so
// refreshMCPToolPolicy reads the new policy inputs.
func (c *Coordinator) applyMCPRegistryChange(ctx context.Context, srv mcpserver.Server) error {
	if srv.Enabled {
		return c.refreshMCPToolPolicy(ctx)
	}
	var projectionErr error
	if c.mcpRegistryCommands != nil {
		projectionErr = c.mcpRegistryCommands.Detach(srv.Name)
	}
	return errors.Join(projectionErr, c.refreshMCPToolPolicy(ctx))
}

// redialMCPServer dispatches a detached live (re)dial for an enabled server whose
// registry change already committed and whose policy already published under the
// mutation lock. The dial runs OUTSIDE that lock (dispatchMCPConnection's task
// blocks on it until the caller's deferred release fires, then dials), so one slow
// endpoint cannot freeze the whole MCP control plane. It reuses the same live
// collaborator the synchronous path used ([mcpRegistryCommands.Configure] with the
// just-committed descriptor); a concurrent reconfigure supersedes a stale dial via
// the adapter's per-server generation. A dial failure does not fail the originating
// call — status surfaces it and it is reconnectable.
func (c *Coordinator) redialMCPServer(ctx context.Context, srv mcpserver.Server) {
	if c.mcpRegistryCommands == nil {
		return
	}
	_ = c.dispatchMCPConnection(ctx, srv.Name, func(dialCtx context.Context) error {
		return c.mcpRegistryCommands.Configure(dialCtx, srv)
	})
}

// TestMCPServer dials srv with a throwaway client and proves its tools list — a
// connection test that touches neither the registry nor the live set, EXCEPT it
// reuses an active OAuth sign-in for the same-named server (so an authorized
// OAuth server tests as connected, not "unauthorized"). Returns the dial /
// tools-list error, or nil on success.
func (c *Coordinator) TestMCPServer(ctx context.Context, input MCPServerInput) (MCPTestResult, error) {
	srv, err := c.validatedMCPServer(ctx, input)
	if err != nil {
		return MCPTestResult{}, err
	}
	if c.mcpRegistryCommands == nil {
		return MCPTestResult{}, ErrUnknownMCPServer
	}
	if err := c.mcpRegistryCommands.Probe(ctx, srv); err != nil {
		return MCPTestResult{}, nil
	}
	return MCPTestResult{OK: true}, nil
}

func (c *Coordinator) validatedMCPServer(ctx context.Context, input MCPServerInput) (mcpserver.Server, error) {
	candidate, err := mcpServerCandidate(input)
	if err != nil {
		return mcpserver.Server{}, err
	}
	return c.resolveMCPServerConfiguration(ctx, candidate)
}

func mcpServerCandidate(input MCPServerInput) (mcpserver.Server, error) {
	srv := input.server()
	err := srv.Validate()
	if err != nil {
		return mcpserver.Server{}, fmt.Errorf("%w: %w", ErrInvalidMCPServerConfiguration, err)
	}
	return srv, nil
}

// MCPTools lists tools advertised by the connected MCP servers (scoped to server
// when non-empty) for mcp.tools.list.
func (c *Coordinator) MCPTools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error) {
	if c.mcpToolCatalog == nil {
		return nil, nil
	}
	return c.mcpToolCatalog.Tools(ctx, server)
}

// refreshMCPToolPolicy atomically publishes the policy derived from the
// just-mutated registry for the next tool resolution and approval decision.
func (c *Coordinator) refreshMCPToolPolicy(ctx context.Context) error {
	servers, err := c.mcpRegistry.List(ctx)
	if err != nil {
		return err
	}
	policy := mcpserver.NewToolPolicy(servers)
	c.mcpPolicy.Replace(policy)
	return nil
}
