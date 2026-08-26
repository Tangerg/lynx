package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/httporigin"
)

const reconcileTimeout = 30 * time.Second

// errClosed reports that a post-commit reconcile / background task could not be
// launched because the component is shutting down.
var errClosed = errors.New("mcp: closed")

// errConnectionUnavailable reports an incomplete coordinator assembly at the
// use-case boundary instead of letting a detached task fail asynchronously.
var errConnectionUnavailable = errors.New("mcp: MCP connection service is unavailable")

// ErrInvalidServerConfiguration marks a malformed MCP configuration command.
// Callers map it to their validation error without re-running domain
// validation or inspecting persistence state.
var ErrInvalidServerConfiguration = errors.New("mcp: invalid MCP server configuration")

// ErrUnknownServer is the application boundary's stable unknown-server
// result. The underlying domain sentinel remains internal to this package.
var ErrUnknownServer = errors.New("mcp: unknown MCP server")

// ErrServerAlreadyExists reports a create whose stable name is already in
// use. Create and update remain distinct operations; storage never guesses from
// an upsert whether the caller meant to replace an existing resource.
var ErrServerAlreadyExists = errors.New("mcp: MCP server already exists")

// ErrServerDisabled reports a connection command against a configured
// server whose durable enablement gate is closed.
var ErrServerDisabled = errors.New("mcp: MCP server is disabled")

// ErrAuthorizationAttemptNotFound reports an unknown or expired interactive
// authorization attempt.
var ErrAuthorizationAttemptNotFound = errors.New("mcp: MCP authorization attempt not found")

// ErrAuthorizationUnsupported reports interactive authorization requested
// for a transport that cannot perform OAuth.
var ErrAuthorizationUnsupported = errors.New("mcp: MCP authorization requires streamable HTTP")

// CreateServer creates one durable resource and projects it into the live MCP
// pool. A duplicate name is a conflict, never an implicit update.
func (c *Coordinator) CreateServer(ctx context.Context, input ServerInput) (Server, error) {
	write, err := c.beginMutation(ctx)
	if err != nil {
		return Server{}, err
	}
	defer write.close()
	if _, found, getErr := c.registry.Get(write.requestCtx, input.Name); getErr != nil {
		return Server{}, getErr
	} else if found {
		return Server{}, ErrServerAlreadyExists
	}
	srv, err := serverCandidate(input, nil)
	if err != nil {
		return Server{}, err
	}
	return c.commitServer(write, srv)
}

// UpdateServer applies an explicit partial update to an existing resource.
// The mutation lock keeps the read/patch/save sequence atomic inside the runtime.
func (c *Coordinator) UpdateServer(ctx context.Context, name string, patch ServerPatch) (Server, error) {
	if patch.Empty() {
		return Server{}, fmt.Errorf("%w: update contains no changes", ErrInvalidServerConfiguration)
	}
	write, err := c.beginMutation(ctx)
	if err != nil {
		return Server{}, err
	}
	defer write.close()
	current, found, err := c.registry.Get(write.requestCtx, name)
	if err != nil {
		return Server{}, err
	}
	if !found {
		return Server{}, ErrUnknownServer
	}
	updated, err := applyServerPatch(current, patch)
	if err != nil {
		return Server{}, err
	}
	return c.commitServer(write, updated)
}

func (c *Coordinator) commitServer(write *mutationScope, srv mcpserver.Server) (Server, error) {
	if err := c.registry.Save(write.requestCtx, srv); err != nil {
		return Server{}, err
	}
	if !srv.Enabled {
		c.cancelDial(srv.Name)
	}
	reconcileCtx, cancel := context.WithTimeout(write.ownerCtx, reconcileTimeout)
	defer cancel()
	reconcileErr := c.applyRegistryChange(reconcileCtx, srv)
	// Make the committed descriptor and its first live state one ordered fact.
	// Enabled servers enter connecting here; their detached dial suppresses its
	// own duplicate connecting event and only publishes the status port's terminal
	// projection. Disabled or unreconciled servers install an unknown tombstone
	// so a stale live entry cannot resurrect the previous connection.
	status := ServerStatus{Name: srv.Name}
	shouldRedial := srv.Enabled && reconcileErr == nil && c.connectionLifecycle != nil
	if shouldRedial {
		status.Known = true
		status.State = mcpserver.ConnectionConnecting
	}
	event := c.prepareStatus(status)
	write.unlock()
	var redialErr error
	var startDial chan struct{}
	if shouldRedial {
		startDial = make(chan struct{})
		// Admit the dial before invoking the event sink. A sink may synchronously
		// reconfigure this server; that newer dial must supersede this one rather
		// than having this older dispatch happen after the callback returns. The
		// gate prevents the admitted task from settling before that callback gets
		// its chance to supersede it.
		redialErr = c.redialServer(write.ownerCtx, srv, startDial)
		if redialErr != nil {
			// Admission can race component shutdown. Reuse this mutation's reserved
			// sequence for the truthful disconnected projection instead of
			// publishing a connecting state that no task can ever settle.
			event.status = ServerStatus{Name: srv.Name}
		}
	}
	c.publishStatus(event)
	if startDial != nil {
		close(startDial)
	}
	if reconcileErr != nil {
		return Server{}, reconcileErr
	}
	if redialErr != nil {
		return Server{}, redialErr
	}
	status, ok := c.statusesByName()[srv.Name]
	if ok {
		return serverView(srv, &status), nil
	}
	return serverView(srv, nil), nil
}

// DeleteServer deletes a server from the registry and drops it from the live
// connections.
func (c *Coordinator) DeleteServer(ctx context.Context, name string) error {
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	if _, found, err := c.registry.Get(write.requestCtx, name); err != nil {
		return err
	} else if !found {
		return ErrUnknownServer
	}
	if err := c.registry.Remove(write.requestCtx, name); err != nil {
		return err
	}
	c.cancelDial(name)
	reconcileCtx, cancel := context.WithTimeout(write.ownerCtx, reconcileTimeout)
	defer cancel()
	// Shrink the live set before publishing the new policy: dropping tools can't
	// expose a hidden one, but publishing first would leave the about-to-be-dropped
	// tools briefly live under the wrong policy.
	var projectionErr error
	if c.connectionLifecycle != nil {
		projectionErr = c.connectionLifecycle.Detach(name)
	}
	policyErr := c.refreshToolPolicy(reconcileCtx)
	event := c.prepareStatus(ServerStatus{Name: name})
	write.unlock()
	c.publishStatus(event)
	return errors.Join(projectionErr, policyErr)
}

// mutationScope owns one registry mutation's request lifetime, detached repair
// lifetime, and serialization lock.
type mutationScope struct {
	coordinator *Coordinator
	requestCtx  context.Context
	ownerCtx    context.Context
	finish      func()
	locked      bool
}

// beginMutation owns both task scopes and the durable mutation lock. Callers
// release the lock before invoking status sinks or dispatching live dials, then
// defer close to release the task scopes on every exit.
func (c *Coordinator) beginMutation(ctx context.Context) (*mutationScope, error) {
	ownerCtx, releaseOwner, ok := c.tasks.Attach(ctx)
	if !ok {
		return nil, errClosed
	}
	requestCtx, releaseRequest, ok := c.tasks.AttachLinked(ctx)
	if !ok {
		releaseOwner()
		return nil, errClosed
	}
	c.mutationMu.Lock()
	if err := requestCtx.Err(); err != nil {
		c.mutationMu.Unlock()
		releaseRequest()
		releaseOwner()
		return nil, err
	}
	return &mutationScope{
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

func (m *mutationScope) unlock() {
	if m != nil && m.locked {
		m.locked = false
		m.coordinator.mutationMu.Unlock()
	}
}

func (m *mutationScope) close() {
	if m == nil || m.finish == nil {
		return
	}
	m.unlock()
	m.finish()
	m.finish = nil
}

// applyRegistryChange reflects a persisted registry entry into the policy
// snapshot and, when disabling, the live tool set — all under the caller's
// mutation lock. Publication order keeps disabled tools from becoming momentarily
// visible:
//   - enabling publishes policy here; the live (re)dial is NOT done here — the
//     caller dispatches it detached, after releasing the lock, because a network
//     handshake must never hold the control-plane lock (see commitServer);
//   - disabling detaches the live projection before publishing policy; physical
//     session retirement remains owned by the live connection lifecycle.
//
// Either reversal would leave a window where a disabled tool is live under the
// wrong policy. The caller has already mutated the registry, so
// refreshToolPolicy reads the new policy inputs.
func (c *Coordinator) applyRegistryChange(ctx context.Context, srv mcpserver.Server) error {
	if srv.Enabled {
		return c.refreshToolPolicy(ctx)
	}
	var projectionErr error
	if c.connectionLifecycle != nil {
		projectionErr = c.connectionLifecycle.Detach(srv.Name)
	}
	return errors.Join(projectionErr, c.refreshToolPolicy(ctx))
}

// redialServer dispatches a detached live (re)dial for an enabled server whose
// registry change already committed and whose policy already published under the
// mutation lock. The dial runs OUTSIDE that lock (dispatchConnection's task
// blocks on it until the caller's deferred release fires, then dials), so one slow
// endpoint cannot freeze the whole MCP control plane. It reuses the same live
// collaborator the synchronous path used ([connectionLifecycle.Configure] with the
// just-committed descriptor); a concurrent reconfigure supersedes a stale dial
// through per-server generation. A dial failure does not fail the originating
// call; status surfaces it and it remains reconnectable.
func (c *Coordinator) redialServer(ctx context.Context, srv mcpserver.Server, start <-chan struct{}) error {
	if c.connectionLifecycle == nil {
		return errConnectionUnavailable
	}
	return c.dispatchConnection(ctx, srv.Name, func(dialCtx context.Context) error {
		return c.connectionLifecycle.Configure(dialCtx, srv)
	}, false, start, nil)
}

// TestServer dials srv with a throwaway client and proves its tools list — a
// connection test that touches neither the registry nor the live set, EXCEPT it
// reuses an active OAuth sign-in for the same-named server (so an authorized
// OAuth server tests as connected, not "unauthorized"). Returns the dial /
// tools-list failure as OK=false; invalid candidates and unavailable registry
// capability are returned as errors.
func (c *Coordinator) TestServer(ctx context.Context, input ServerInput) (TestResult, error) {
	srv, err := c.validatedServer(ctx, input)
	if err != nil {
		return TestResult{}, err
	}
	if c.connectionLifecycle == nil {
		return TestResult{}, ErrUnknownServer
	}
	if err := c.connectionLifecycle.Probe(ctx, srv); err != nil {
		return TestResult{}, nil
	}
	return TestResult{OK: true}, nil
}

func (c *Coordinator) validatedServer(ctx context.Context, input ServerInput) (mcpserver.Server, error) {
	var current *mcpserver.Server
	if c.registry != nil && input.Name != "" {
		stored, found, err := c.registry.Get(ctx, input.Name)
		if err != nil {
			return mcpserver.Server{}, err
		}
		if found {
			current = &stored
		}
	}
	return serverCandidate(input, current)
}

func serverCandidate(input ServerInput, current *mcpserver.Server) (mcpserver.Server, error) {
	connection, err := resolveConnection(input.Connection, current)
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
		return mcpserver.Server{}, fmt.Errorf("%w: %w", ErrInvalidServerConfiguration, err)
	}
	return srv, nil
}

func applyServerPatch(current mcpserver.Server, patch ServerPatch) (mcpserver.Server, error) {
	updated := current
	if patch.Enabled != nil {
		updated.Enabled = *patch.Enabled
	}
	if patch.Description != nil {
		updated.Description = *patch.Description
	}
	if patch.Connection != nil {
		connection, err := resolveConnection(*patch.Connection, &current)
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
		return mcpserver.Server{}, fmt.Errorf("%w: %w", ErrInvalidServerConfiguration, err)
	}
	return updated, nil
}

func resolveConnection(input ConnectionInput, current *mcpserver.Server) (mcpserver.Server, error) {
	connection := mcpserver.Server{
		Transport: input.Transport,
		URL:       input.URL,
		Command:   input.Command,
		Args:      slices.Clone(input.Args),
		Dir:       input.Dir,
	}
	switch input.Transport {
	case mcpserver.TransportStreamableHTTP:
		return resolveHTTPConnection(input, current, connection)
	case mcpserver.TransportStdio:
		return resolveStdioConnection(input, current, connection)
	default:
		return mcpserver.Server{}, fmt.Errorf("%w: unknown transport %q", ErrInvalidServerConfiguration, input.Transport)
	}
}

func resolveHTTPConnection(
	input ConnectionInput,
	current *mcpserver.Server,
	connection mcpserver.Server,
) (mcpserver.Server, error) {
	if input.Environment != nil {
		return mcpserver.Server{}, fmt.Errorf(
			"%w: environment applies to stdio transport only",
			ErrInvalidServerConfiguration,
		)
	}
	if _, err := httporigin.Parse(input.URL); err != nil {
		return mcpserver.Server{}, fmt.Errorf(
			"%w: invalid HTTP endpoint: %w",
			ErrInvalidServerConfiguration,
			err,
		)
	}
	sameOrigin := current != nil &&
		current.Transport == mcpserver.TransportStreamableHTTP &&
		httporigin.Same(current.URL, input.URL)
	authorization, err := resolveAuthorization(input.Authorization, current, sameOrigin)
	if err != nil {
		return mcpserver.Server{}, err
	}
	headers, err := resolveHeaders(input.Headers, current, sameOrigin)
	if err != nil {
		return mcpserver.Server{}, err
	}
	connection.Authorization = authorization
	connection.Headers = headers
	return connection, nil
}

func resolveAuthorization(
	change *AuthorizationChange,
	current *mcpserver.Server,
	mayInherit bool,
) (string, error) {
	switch {
	case change == nil && mayInherit:
		return current.Authorization, nil
	case change == nil && current != nil && current.Authorization != "":
		return "", fmt.Errorf(
			"%w: changing the HTTP origin requires authorization to be explicitly set or cleared",
			ErrInvalidServerConfiguration,
		)
	case change == nil:
		return "", nil
	case change.Kind == SecretSet && change.Value == "":
		return "", fmt.Errorf("%w: authorization set value is empty", ErrInvalidServerConfiguration)
	case change.Kind == SecretSet:
		return change.Value, nil
	case change.Kind == SecretClear && current == nil:
		return "", fmt.Errorf(
			"%w: authorization clear requires an existing server",
			ErrInvalidServerConfiguration,
		)
	case change.Kind == SecretClear:
		return "", nil
	default:
		return "", fmt.Errorf("%w: unknown authorization change", ErrInvalidServerConfiguration)
	}
}

func resolveHeaders(
	change *HeadersChange,
	current *mcpserver.Server,
	mayInherit bool,
) (map[string]string, error) {
	switch {
	case change == nil && mayInherit:
		return maps.Clone(current.Headers), nil
	case change == nil && current != nil && len(current.Headers) > 0:
		return nil, fmt.Errorf(
			"%w: changing the HTTP origin requires headers to be explicitly set or cleared",
			ErrInvalidServerConfiguration,
		)
	case change == nil:
		return nil, nil
	case change.Kind == SecretSet && len(change.Value) == 0:
		return nil, fmt.Errorf("%w: headers set value is empty", ErrInvalidServerConfiguration)
	case change.Kind == SecretSet:
		return maps.Clone(change.Value), nil
	case change.Kind == SecretClear && current == nil:
		return nil, fmt.Errorf(
			"%w: headers clear requires an existing server",
			ErrInvalidServerConfiguration,
		)
	case change.Kind == SecretClear:
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: unknown headers change", ErrInvalidServerConfiguration)
	}
}

func resolveStdioConnection(
	input ConnectionInput,
	current *mcpserver.Server,
	connection mcpserver.Server,
) (mcpserver.Server, error) {
	if input.Authorization != nil || input.Headers != nil {
		return mcpserver.Server{}, fmt.Errorf(
			"%w: authorization and headers apply to HTTP transport only",
			ErrInvalidServerConfiguration,
		)
	}
	sameTarget := current != nil &&
		current.Transport == mcpserver.TransportStdio &&
		current.Command == input.Command &&
		slices.Equal(current.Args, input.Args) &&
		current.Dir == input.Dir
	environment, err := resolveEnvironment(input.Environment, current, sameTarget)
	if err != nil {
		return mcpserver.Server{}, err
	}
	connection.Env = environment
	return connection, nil
}

func resolveEnvironment(
	change *EnvironmentChange,
	current *mcpserver.Server,
	mayInherit bool,
) (map[string]string, error) {
	switch {
	case change == nil && mayInherit:
		return maps.Clone(current.Env), nil
	case change == nil && current != nil && len(current.Env) > 0:
		return nil, fmt.Errorf(
			"%w: changing the stdio process target requires environment variables to be explicitly set or cleared",
			ErrInvalidServerConfiguration,
		)
	case change == nil:
		return nil, nil
	case change.Kind == SecretSet && len(change.Value) == 0:
		return nil, fmt.Errorf("%w: environment set value is empty", ErrInvalidServerConfiguration)
	case change.Kind == SecretSet:
		return maps.Clone(change.Value), nil
	case change.Kind == SecretClear && current == nil:
		return nil, fmt.Errorf(
			"%w: environment clear requires an existing server",
			ErrInvalidServerConfiguration,
		)
	case change.Kind == SecretClear:
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: unknown environment change", ErrInvalidServerConfiguration)
	}
}

// Tools lists tools advertised by the connected MCP servers (scoped to server
// when non-empty) for tool discovery.
func (c *Coordinator) Tools(ctx context.Context, server string) ([]mcpserver.AdvertisedTool, error) {
	if c.toolCatalog == nil {
		return nil, nil
	}
	return c.toolCatalog.Tools(ctx, server)
}

// refreshToolPolicy atomically publishes the policy derived from the
// just-mutated registry for the next tool resolution and approval decision.
func (c *Coordinator) refreshToolPolicy(ctx context.Context) error {
	servers, err := c.registry.List(ctx)
	if err != nil {
		return err
	}
	policy := mcpserver.NewToolPolicy(servers)
	c.policy.Replace(policy)
	return nil
}
