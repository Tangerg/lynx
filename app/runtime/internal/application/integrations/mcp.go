package integrations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"

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

// MCP-server registry orchestration: the coordinator owns both the persisted
// registry and the live connection pool, so a
// configure/remove/enable both persists and applies to the live tool set in one
// place. Registry entries are projected to dial-level descriptors only at the
// live-connection boundary.

// ListMCPServerConfigs returns safe editable MCP configurations. The durable
// domain entries never cross the application boundary because they carry raw
// authorization tokens.
func (c *Coordinator) ListMCPServerConfigs(ctx context.Context) ([]MCPServerConfig, error) {
	if c.mcpRegistry == nil {
		return nil, errors.New("integrations: MCP registry is unavailable")
	}
	servers, err := c.mcpRegistry.List(ctx)
	if err != nil {
		return nil, err
	}
	configs := make([]MCPServerConfig, 0, len(servers))
	for _, server := range servers {
		configs = append(configs, mcpConfigView(server))
	}
	return configs, nil
}

// MCPServerStatuses resolves the safe live status read model for every tracked
// server. Raw adapter errors are intentionally reduced to stable diagnostics.
func (c *Coordinator) MCPServerStatuses(ctx context.Context) []MCPServerStatus {
	if c.mcpStatusReader == nil {
		return nil
	}
	statuses := c.mcpStatusReader.Statuses()
	views := make([]MCPServerStatus, 0, len(statuses))
	for _, status := range statuses {
		views = append(views, c.mcpStatusView(ctx, status))
	}
	return views
}

// MCPServerStatus resolves one safe live status read model.
func (c *Coordinator) MCPServerStatus(ctx context.Context, name string) MCPServerStatus {
	if c.mcpStatusReader == nil {
		return MCPServerStatus{Name: name}
	}
	for _, status := range c.mcpStatusReader.Statuses() {
		if status.Name == name {
			return c.mcpStatusView(ctx, status)
		}
	}
	return MCPServerStatus{Name: name}
}

func (c *Coordinator) mcpStatusView(ctx context.Context, status mcpserver.ConnectionStatus) MCPServerStatus {
	var toolCount *int
	if status.State == mcpserver.ConnectionConnected && c.mcpToolCatalog != nil {
		if tools, err := c.mcpToolCatalog.Tools(ctx, status.Name); err == nil {
			count := len(tools)
			toolCount = &count
		}
	}
	return mcpStatusView(status, toolCount)
}

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

// ReconnectMCPServer re-dials a configured MCP server and hot-swaps the live tool
// set (mcp.servers.reconnect). Fire-and-forget: the name is validated
// synchronously (unknown → [mcpserver.ErrUnknownServer]), then the dial runs on
// the component task group with connecting → settled status published for the
// workspace stream, so a returning RPC does not abort it while shutdown still can.
func (c *Coordinator) ReconnectMCPServer(ctx context.Context, name string) error {
	return c.startMCPConnection(ctx, name, func(ctx context.Context) error {
		return c.mcpConnectionCommands.Reconnect(ctx, name)
	})
}

// AuthorizeMCPServer runs the interactive OAuth sign-in for an HTTP MCP server
// (mcp.servers.authorize) — opens the system browser, catches the loopback
// redirect, and connects on success. Fire-and-forget like reconnect; the
// credentials live for the process only (re-prompt after restart).
func (c *Coordinator) AuthorizeMCPServer(ctx context.Context, name string) error {
	return c.startMCPConnection(ctx, name, func(ctx context.Context) error {
		return c.mcpConnectionCommands.Authorize(ctx, name)
	})
}

// startMCPConnection validates the server exists, then runs dial on the
// component task group — detached from the caller's cancellation (so a returning
// RPC cannot abort it) but keeping its trace values and canceled + joined by
// Close. It enters the application mutation order only for the pre/post registry
// checks and status publication; the dial itself runs outside that global
// critical section. The live adapter's per-server generation makes a concurrent
// configure/remove supersede stale dial completion, while unrelated servers can
// connect in parallel. The task's context scopes both registry reads and dial.
// Returns [errMCPConnectionUnavailable] when the coordinator lacks a required
// connection dependency, [mcpserver.ErrUnknownServer] for an unknown name (the
// delivery layer maps it to invalid_params), or [errClosed] during shutdown.
func (c *Coordinator) startMCPConnection(ctx context.Context, name string, dial func(context.Context) error) error {
	if c.mcpRegistry == nil || c.mcpStatusReader == nil || c.mcpConnectionCommands == nil {
		return errMCPConnectionUnavailable
	}
	if !c.mcpServerKnown(name) {
		return ErrUnknownMCPServer
	}
	return c.dispatchMCPConnection(ctx, name, dial)
}

// dispatchMCPConnection runs a live (re)dial on the component task group, detached
// from the caller's cancellation. It enters the mutation order only for the
// pre/post registry checks and status publication; the dial itself runs OUTSIDE
// that global critical section, so a slow endpoint cannot freeze the control
// plane, and the registry re-read lets a concurrent configure/remove supersede a
// stale completion. A caller that already holds mcpMutationMu may invoke this: the
// spawned task blocks on the lock until that caller releases it, then proceeds —
// which is exactly how the registry-write methods dispatch their live dial without
// holding the lock across the network handshake. Returns errClosed only when the
// task group is shutting down.
func (c *Coordinator) dispatchMCPConnection(ctx context.Context, name string, connect func(context.Context) error) error {
	ownerCtx, releaseOwner, ok := c.tasks.Attach(ctx)
	if !ok {
		return errClosed
	}
	dialCtx, operation := c.replaceMCPDial(ownerCtx, name)
	if !c.tasks.StartLinked(dialCtx, func(ctx context.Context) {
		defer releaseOwner()
		defer c.clearMCPDial(name, operation)
		if err := ctx.Err(); err != nil {
			return
		}
		c.mcpMutationMu.Lock()
		srv, ok, err := c.mcpRegistry.Get(ctx, name)
		if err != nil {
			c.mcpMutationMu.Unlock()
			recordMCPConnectionError(ctx, fmt.Errorf("integrations: read MCP server %q before connection: %w", name, err))
			return
		}
		if !ok || !srv.Enabled {
			c.mcpMutationMu.Unlock()
			return
		}
		c.notifyMCPStatus(ctx, name, true)
		c.mcpMutationMu.Unlock()

		// Interactive OAuth may wait minutes for a human. The live connection
		// adapter owns per-server generation/cancellation, so no application-wide
		// mutation lock is held while dialing. A configure/remove can supersede it
		// immediately; stale adapter completion cannot swap itself back in.
		_ = connect(ctx)

		c.mcpMutationMu.Lock()
		defer c.mcpMutationMu.Unlock()
		srv, ok, err = c.mcpRegistry.Get(ctx, name)
		if err != nil {
			recordMCPConnectionError(ctx, fmt.Errorf("integrations: read MCP server %q after connection: %w", name, err))
			return
		}
		if !ok || !srv.Enabled {
			// Defensive projection cleanup for adapters that cannot cancel a stale
			// operation themselves. The production adapter rejects stale generations,
			// so this is normally an idempotent no-op.
			if c.mcpRegistryCommands != nil {
				c.mcpRegistryCommands.Remove(ctx, name)
			}
			return
		}
		c.notifyMCPStatus(ctx, name, false)
	}) {
		operation.cancel()
		c.clearMCPDial(name, operation)
		releaseOwner()
		return errClosed
	}
	return nil
}

// replaceMCPDial gives each server exactly one current connection operation.
// A registry mutation, reconnect, or authorize supersedes the previous dial by
// canceling its context; adapters must honor ctx while dialing. The generation
// check after a dial remains a defensive cleanup for adapters whose transport
// cannot stop synchronously.
func (c *Coordinator) replaceMCPDial(ctx context.Context, name string) (context.Context, *mcpDial) {
	dialCtx, cancel := context.WithCancel(ctx)
	dial := &mcpDial{cancel: cancel}
	c.mcpDialMu.Lock()
	if previous := c.mcpDials[name]; previous != nil {
		previous.cancel()
	}
	c.mcpDials[name] = dial
	c.mcpDialMu.Unlock()
	return dialCtx, dial
}

func (c *Coordinator) cancelMCPDial(name string) {
	c.mcpDialMu.Lock()
	if dial := c.mcpDials[name]; dial != nil {
		dial.cancel()
		delete(c.mcpDials, name)
	}
	c.mcpDialMu.Unlock()
}

func (c *Coordinator) clearMCPDial(name string, dial *mcpDial) {
	c.mcpDialMu.Lock()
	if c.mcpDials[name] == dial {
		delete(c.mcpDials, name)
	}
	c.mcpDialMu.Unlock()
}

func recordMCPConnectionError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}

// mcpServerKnown reports whether name is a tracked MCP server (a configured
// server appears in the live statuses even when its last dial failed).
func (c *Coordinator) mcpServerKnown(name string) bool {
	if c.mcpStatusReader == nil {
		return false
	}
	for _, st := range c.mcpStatusReader.Statuses() {
		if st.Name == name {
			return true
		}
	}
	return false
}

func (c *Coordinator) notifyMCPStatus(ctx context.Context, name string, connecting bool) {
	if c.mcpStatus != nil {
		if connecting {
			c.mcpStatus(MCPServerStatus{Name: name, Known: true, State: MCPConnecting})
			return
		}
		c.mcpStatus(c.MCPServerStatus(ctx, name))
	}
}

// ConfigureMCPServer upserts a server in the registry and applies it to the live
// connections: an enabled server is (re)dialed, a disabled one is dropped from
// the live set (it stays in the registry). A dial failure does not fail the call
// — the server is persisted and tracked "failed" (reconnectable); the
// connectivity feedback path is TestMCPServer.
func (c *Coordinator) ConfigureMCPServer(ctx context.Context, input MCPServerInput) (MCPServerConfig, error) {
	srv, err := c.resolveMCPServerConfiguration(ctx, input.server())
	if err != nil {
		return MCPServerConfig{}, err
	}
	if err := srv.Validate(); err != nil {
		return MCPServerConfig{}, fmt.Errorf("%w: %w", ErrInvalidMCPServerConfiguration, err)
	}
	requestCtx, ownerCtx, release, err := c.beginMCPWrite(ctx)
	if err != nil {
		return MCPServerConfig{}, err
	}
	defer release()
	if err := c.mcpRegistry.Configure(requestCtx, srv); err != nil {
		return MCPServerConfig{}, err
	}
	reconcileCtx, cancel := context.WithTimeout(ownerCtx, mcpReconcileTimeout)
	defer cancel()
	if err := c.applyMCPRegistryChange(reconcileCtx, srv); err != nil {
		return MCPServerConfig{}, err
	}
	c.notifyMCPStatus(ownerCtx, srv.Name, false)
	if srv.Enabled {
		c.redialMCPServer(ownerCtx, srv)
	}
	return mcpConfigView(srv), nil
}

// RemoveMCPServer deletes a server from the registry and drops it from the live
// connections.
func (c *Coordinator) RemoveMCPServer(ctx context.Context, name string) error {
	requestCtx, ownerCtx, release, err := c.beginMCPWrite(ctx)
	if err != nil {
		return err
	}
	defer release()
	c.cancelMCPDial(name)
	if err := c.mcpRegistry.Remove(requestCtx, name); err != nil {
		return err
	}
	reconcileCtx, cancel := context.WithTimeout(ownerCtx, mcpReconcileTimeout)
	defer cancel()
	// Shrink the live set before publishing the new policy: dropping tools can't
	// expose a hidden one, but publishing first would leave the about-to-be-dropped
	// tools briefly live under the wrong policy.
	if c.mcpRegistryCommands != nil {
		c.mcpRegistryCommands.Remove(reconcileCtx, name)
	}
	if err := c.refreshMCPToolPolicy(reconcileCtx); err != nil {
		return err
	}
	c.notifyMCPStatus(ownerCtx, name, false)
	return nil
}

// SetMCPServerEnabled flips a server's enablement in the registry and applies it
// to the live connections (enable → dial, disable → drop).
func (c *Coordinator) SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error {
	requestCtx, ownerCtx, release, err := c.beginMCPWrite(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := c.mcpRegistry.SetEnabled(requestCtx, name, enabled); err != nil {
		return err
	}
	reconcileCtx, cancel := context.WithTimeout(ownerCtx, mcpReconcileTimeout)
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
	c.notifyMCPStatus(ownerCtx, srv.Name, false)
	if srv.Enabled {
		c.redialMCPServer(ownerCtx, srv)
	}
	return nil
}

// beginMCPMutation gives a write both scopes it needs: requestCtx is canceled by
// the caller or component shutdown and is used until the durable registry commit;
// ownerCtx ignores caller cancellation but is still canceled by shutdown, so
// post-commit live/policy reconciliation cannot be abandoned by a dropped
// connection or escape component shutdown.
func (c *Coordinator) beginMCPMutation(parent context.Context) (
	requestCtx context.Context,
	ownerCtx context.Context,
	finish func(),
	err error,
) {
	ownerCtx, releaseOwner, ok := c.tasks.Attach(parent)
	if !ok {
		return nil, nil, nil, errClosed
	}
	requestCtx, releaseRequest, ok := c.tasks.AttachLinked(parent)
	if !ok {
		releaseOwner()
		return nil, nil, nil, errClosed
	}
	return requestCtx, ownerCtx, func() {
		releaseRequest()
		releaseOwner()
	}, nil
}

// beginMCPWrite acquires both mutation scopes plus the serialization lock and
// returns a single release the caller must defer. requestCtx is caller-cancelable
// (used through the durable registry commit); ownerCtx survives caller cancel but
// not component shutdown (used for post-commit live/policy reconciliation).
// release unlocks then tears the scopes down — the acquire order reversed — so the
// lock/ctx/teardown protocol lives in one place across the write methods. err is
// non-nil when the component is shutting down or the caller ctx is already dead.
func (c *Coordinator) beginMCPWrite(ctx context.Context) (requestCtx, ownerCtx context.Context, release func(), err error) {
	requestCtx, ownerCtx, finish, err := c.beginMCPMutation(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	c.mcpMutationMu.Lock()
	if err := requestCtx.Err(); err != nil {
		c.mcpMutationMu.Unlock()
		finish()
		return nil, nil, nil, err
	}
	return requestCtx, ownerCtx, func() {
		c.mcpMutationMu.Unlock()
		finish()
	}, nil
}

// applyMCPRegistryChange reflects a persisted registry entry into the policy
// snapshot and, when disabling, the live tool set — all under the caller's
// mutation lock. Publication order keeps disabled tools from becoming momentarily
// visible:
//   - enabling publishes policy here; the live (re)dial is NOT done here — the
//     caller dispatches it detached, after releasing the lock, because a network
//     handshake must never hold the control-plane lock (see ConfigureMCPServer);
//   - disabling removes tools (bounded teardown) before publishing policy.
//
// Either reversal would leave a window where a disabled tool is live under the
// wrong policy. The caller has already mutated the registry, so
// refreshMCPToolPolicy reads the new policy inputs.
func (c *Coordinator) applyMCPRegistryChange(ctx context.Context, srv mcpserver.Server) error {
	if srv.Enabled {
		return c.refreshMCPToolPolicy(ctx)
	}
	if c.mcpRegistryCommands != nil {
		c.mcpRegistryCommands.Remove(ctx, srv.Name)
	}
	return c.refreshMCPToolPolicy(ctx)
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
		return c.mcpRegistryCommands.Configure(dialCtx, mcpserver.ConfigFromServer(srv))
	})
}

// TestMCPServer dials srv with a throwaway client and proves its tools list — a
// connection test that touches neither the registry nor the live set, EXCEPT it
// reuses an active OAuth sign-in for the same-named server (so an authorized
// OAuth server tests as connected, not "unauthorized"). Returns the dial /
// tools-list error, or nil on success.
func (c *Coordinator) TestMCPServer(ctx context.Context, input MCPServerInput) (MCPTestResult, error) {
	srv, err := c.resolveMCPServerConfiguration(ctx, input.server())
	if err != nil {
		return MCPTestResult{}, err
	}
	if err := srv.Validate(); err != nil {
		return MCPTestResult{}, fmt.Errorf("%w: %w", ErrInvalidMCPServerConfiguration, err)
	}
	if c.mcpRegistryCommands == nil {
		return MCPTestResult{}, ErrUnknownMCPServer
	}
	if err := c.mcpRegistryCommands.Probe(ctx, mcpserver.ConfigFromServer(srv)); err != nil {
		return MCPTestResult{Problem: &MCPProblem{Type: "mcp_dial_failed", Detail: "MCP connection test failed."}}, nil
	}
	return MCPTestResult{OK: true}, nil
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
