package integrations

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
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
// Callers map it to their validation error without re-running domain
// validation or inspecting persistence state.
var ErrInvalidMCPServerConfiguration = errors.New("integrations: invalid MCP server configuration")

// ErrUnknownMCPServer is the application boundary's stable unknown-server
// result. The underlying domain sentinel remains internal to this package.
var ErrUnknownMCPServer = errors.New("integrations: unknown MCP server")

// ErrMCPServerAlreadyExists reports a create whose stable name is already in
// use. Create and update remain distinct operations; storage never guesses from
// an upsert whether the caller meant to replace an existing resource.
var ErrMCPServerAlreadyExists = errors.New("integrations: MCP server already exists")

// ErrMCPServerDisabled reports a connection command against a configured
// server whose durable enablement gate is closed.
var ErrMCPServerDisabled = errors.New("integrations: MCP server is disabled")

// ErrMCPAuthorizationAttemptNotFound reports an unknown or expired interactive
// authorization attempt.
var ErrMCPAuthorizationAttemptNotFound = errors.New("integrations: MCP authorization attempt not found")

// ErrMCPAuthorizationUnsupported reports interactive authorization requested
// for a transport that cannot perform OAuth.
var ErrMCPAuthorizationUnsupported = errors.New("integrations: MCP authorization requires streamable HTTP")

// CreateMCPServer creates one durable resource and projects it into the live MCP
// pool. A duplicate name is a conflict, never an implicit update.
func (c *Coordinator) CreateMCPServer(ctx context.Context, input MCPServerInput) (MCPServer, error) {
	write, err := c.beginMCPWrite(ctx)
	if err != nil {
		return MCPServer{}, err
	}
	defer write.close()
	if _, found, err := c.mcpRegistry.Get(write.requestCtx, input.Name); err != nil {
		return MCPServer{}, err
	} else if found {
		return MCPServer{}, ErrMCPServerAlreadyExists
	}
	srv, err := mcpServerCandidate(input, nil)
	if err != nil {
		return MCPServer{}, err
	}
	return c.commitMCPServer(write, srv)
}

// UpdateMCPServer applies an explicit partial update to an existing resource.
// The mutation lock keeps the read/patch/save sequence atomic inside the runtime.
func (c *Coordinator) UpdateMCPServer(ctx context.Context, name string, patch MCPServerPatch) (MCPServer, error) {
	if patch.Empty() {
		return MCPServer{}, fmt.Errorf("%w: update contains no changes", ErrInvalidMCPServerConfiguration)
	}
	write, err := c.beginMCPWrite(ctx)
	if err != nil {
		return MCPServer{}, err
	}
	defer write.close()
	current, found, err := c.mcpRegistry.Get(write.requestCtx, name)
	if err != nil {
		return MCPServer{}, err
	}
	if !found {
		return MCPServer{}, ErrUnknownMCPServer
	}
	updated, err := applyMCPServerPatch(current, patch)
	if err != nil {
		return MCPServer{}, err
	}
	return c.commitMCPServer(write, updated)
}

func (c *Coordinator) commitMCPServer(write *mcpWrite, srv mcpserver.Server) (MCPServer, error) {
	if err := c.mcpRegistry.Save(write.requestCtx, srv); err != nil {
		return MCPServer{}, err
	}
	if !srv.Enabled {
		c.cancelMCPDial(srv.Name)
	}
	reconcileCtx, cancel := context.WithTimeout(write.ownerCtx, mcpReconcileTimeout)
	defer cancel()
	if err := c.applyMCPRegistryChange(reconcileCtx, srv); err != nil {
		return MCPServer{}, err
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
	status, ok := c.mcpStatusesByName()[srv.Name]
	if ok {
		return mcpServerView(srv, &status), nil
	}
	return mcpServerView(srv, nil), nil
}

// DeleteMCPServer deletes a server from the registry and drops it from the live
// connections.
func (c *Coordinator) DeleteMCPServer(ctx context.Context, name string) error {
	write, err := c.beginMCPWrite(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	if _, found, err := c.mcpRegistry.Get(write.requestCtx, name); err != nil {
		return err
	} else if !found {
		return ErrUnknownMCPServer
	}
	if err := c.mcpRegistry.Remove(write.requestCtx, name); err != nil {
		return err
	}
	c.cancelMCPDial(name)
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
//     handshake must never hold the control-plane lock (see commitMCPServer);
//   - disabling detaches the live projection before publishing policy; physical
//     session retirement remains owned by the live connection lifecycle.
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
// just-committed descriptor); a concurrent reconfigure supersedes a stale dial
// through per-server generation. A dial failure does not fail the originating
// call; status surfaces it and it remains reconnectable.
func (c *Coordinator) redialMCPServer(ctx context.Context, srv mcpserver.Server) {
	if c.mcpRegistryCommands == nil {
		return
	}
	_ = c.dispatchMCPConnection(ctx, srv.Name, func(dialCtx context.Context) error {
		return c.mcpRegistryCommands.Configure(dialCtx, srv)
	}, nil)
}

// TestMCPServer dials srv with a throwaway client and proves its tools list — a
// connection test that touches neither the registry nor the live set, EXCEPT it
// reuses an active OAuth sign-in for the same-named server (so an authorized
// OAuth server tests as connected, not "unauthorized"). Returns the dial /
// tools-list failure as OK=false; invalid candidates and unavailable registry
// capability are returned as errors.
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
	var current *mcpserver.Server
	if c.mcpRegistry != nil && input.Name != "" {
		stored, found, err := c.mcpRegistry.Get(ctx, input.Name)
		if err != nil {
			return mcpserver.Server{}, err
		}
		if found {
			current = &stored
		}
	}
	return mcpServerCandidate(input, current)
}

func mcpServerCandidate(input MCPServerInput, current *mcpserver.Server) (mcpserver.Server, error) {
	connection, err := resolveMCPConnection(input.Connection, current)
	if err != nil {
		return mcpserver.Server{}, err
	}
	srv := mcpserver.Server{
		Name:             input.Name,
		Transport:        connection.Transport,
		Enabled:          input.Enabled,
		Description:      input.Description,
		URL:              connection.URL,
		Authorization:    connection.Authorization,
		Headers:          connection.Headers,
		Command:          connection.Command,
		Args:             connection.Args,
		Env:              connection.Env,
		Dir:              connection.Dir,
		Timeout:          input.Timeout,
		DisabledTools:    slices.Clone(input.DisabledTools),
		AutoApproveTools: slices.Clone(input.AutoApproveTools),
	}
	if err := srv.Validate(); err != nil {
		return mcpserver.Server{}, fmt.Errorf("%w: %w", ErrInvalidMCPServerConfiguration, err)
	}
	return srv, nil
}

func applyMCPServerPatch(current mcpserver.Server, patch MCPServerPatch) (mcpserver.Server, error) {
	updated := current
	if patch.Enabled != nil {
		updated.Enabled = *patch.Enabled
	}
	if patch.Description != nil {
		updated.Description = *patch.Description
	}
	if patch.Connection != nil {
		connection, err := resolveMCPConnection(*patch.Connection, &current)
		if err != nil {
			return mcpserver.Server{}, err
		}
		updated.Transport = connection.Transport
		updated.URL = connection.URL
		updated.Authorization = connection.Authorization
		updated.Headers = connection.Headers
		updated.Command = connection.Command
		updated.Args = connection.Args
		updated.Env = connection.Env
		updated.Dir = connection.Dir
	}
	if patch.Timeout != nil {
		updated.Timeout = *patch.Timeout
	}
	if patch.DisabledTools != nil {
		updated.DisabledTools = slices.Clone(*patch.DisabledTools)
	}
	if patch.AutoApproveTools != nil {
		updated.AutoApproveTools = slices.Clone(*patch.AutoApproveTools)
	}
	if err := updated.Validate(); err != nil {
		return mcpserver.Server{}, fmt.Errorf("%w: %w", ErrInvalidMCPServerConfiguration, err)
	}
	return updated, nil
}

func resolveMCPConnection(input MCPConnectionInput, current *mcpserver.Server) (mcpserver.Server, error) {
	connection := mcpserver.Server{
		Transport: input.Transport,
		URL:       input.URL,
		Command:   input.Command,
		Args:      slices.Clone(input.Args),
		Dir:       input.Dir,
	}
	switch input.Transport {
	case mcpserver.TransportStreamableHTTP:
		if input.Environment != nil {
			return mcpserver.Server{}, fmt.Errorf("%w: environment applies to stdio transport only", ErrInvalidMCPServerConfiguration)
		}
		if _, err := httporigin.Parse(input.URL); err != nil {
			return mcpserver.Server{}, fmt.Errorf("%w: invalid HTTP endpoint: %w", ErrInvalidMCPServerConfiguration, err)
		}
		sameOrigin := current != nil &&
			current.Transport == mcpserver.TransportStreamableHTTP &&
			httporigin.Same(current.URL, input.URL)
		switch {
		case input.Authorization == nil:
			if sameOrigin {
				connection.Authorization = current.Authorization
			} else if current != nil && current.Authorization != "" {
				return mcpserver.Server{}, fmt.Errorf(
					"%w: changing the HTTP origin requires authorization to be explicitly set or cleared",
					ErrInvalidMCPServerConfiguration,
				)
			}
		case input.Authorization.Kind == MCPSecretSet:
			if input.Authorization.Value == "" {
				return mcpserver.Server{}, fmt.Errorf("%w: authorization set value is empty", ErrInvalidMCPServerConfiguration)
			}
			connection.Authorization = input.Authorization.Value
		case input.Authorization.Kind == MCPSecretClear:
			if current == nil {
				return mcpserver.Server{}, fmt.Errorf("%w: authorization clear requires an existing server", ErrInvalidMCPServerConfiguration)
			}
		default:
			return mcpserver.Server{}, fmt.Errorf("%w: unknown authorization change", ErrInvalidMCPServerConfiguration)
		}
		switch {
		case input.Headers == nil:
			if sameOrigin {
				connection.Headers = maps.Clone(current.Headers)
			} else if current != nil && len(current.Headers) > 0 {
				return mcpserver.Server{}, fmt.Errorf(
					"%w: changing the HTTP origin requires headers to be explicitly set or cleared",
					ErrInvalidMCPServerConfiguration,
				)
			}
		case input.Headers.Kind == MCPSecretSet:
			if len(input.Headers.Value) == 0 {
				return mcpserver.Server{}, fmt.Errorf("%w: headers set value is empty", ErrInvalidMCPServerConfiguration)
			}
			connection.Headers = maps.Clone(input.Headers.Value)
		case input.Headers.Kind == MCPSecretClear:
			if current == nil {
				return mcpserver.Server{}, fmt.Errorf("%w: headers clear requires an existing server", ErrInvalidMCPServerConfiguration)
			}
		default:
			return mcpserver.Server{}, fmt.Errorf("%w: unknown headers change", ErrInvalidMCPServerConfiguration)
		}
	case mcpserver.TransportStdio:
		if input.Authorization != nil || input.Headers != nil {
			return mcpserver.Server{}, fmt.Errorf("%w: authorization and headers apply to HTTP transport only", ErrInvalidMCPServerConfiguration)
		}
		sameTarget := current != nil &&
			current.Transport == mcpserver.TransportStdio &&
			current.Command == input.Command &&
			slices.Equal(current.Args, input.Args) &&
			current.Dir == input.Dir
		switch {
		case input.Environment == nil:
			if sameTarget {
				connection.Env = maps.Clone(current.Env)
			} else if current != nil && len(current.Env) > 0 {
				return mcpserver.Server{}, fmt.Errorf(
					"%w: changing the stdio process target requires environment variables to be explicitly set or cleared",
					ErrInvalidMCPServerConfiguration,
				)
			}
		case input.Environment.Kind == MCPSecretSet:
			if len(input.Environment.Value) == 0 {
				return mcpserver.Server{}, fmt.Errorf("%w: environment set value is empty", ErrInvalidMCPServerConfiguration)
			}
			connection.Env = maps.Clone(input.Environment.Value)
		case input.Environment.Kind == MCPSecretClear:
			if current == nil {
				return mcpserver.Server{}, fmt.Errorf("%w: environment clear requires an existing server", ErrInvalidMCPServerConfiguration)
			}
		default:
			return mcpserver.Server{}, fmt.Errorf("%w: unknown environment change", ErrInvalidMCPServerConfiguration)
		}
	default:
		return mcpserver.Server{}, fmt.Errorf("%w: unknown transport %q", ErrInvalidMCPServerConfiguration, input.Transport)
	}
	return connection, nil
}

// MCPTools lists tools advertised by the connected MCP servers (scoped to server
// when non-empty) for tool discovery.
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
