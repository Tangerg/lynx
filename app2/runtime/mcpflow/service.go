// Package mcpflow owns MCP configuration use cases and generation-safe live
// transport sessions. Secrets cross only the write/store/dial boundary and are
// never projected back to callers.
package mcpflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app2/runtime/domain/mcpserver"
	"github.com/Tangerg/lynx/app2/runtime/internal/identitylane"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const (
	defaultTimeout     = 15 * time.Second
	maximumPatchRetries = 8
	maximumRemoteTools = 2_048
	maximumRemoteSchemaBytes = 1 << 20
	maximumRemoteDescriptionBytes = 64 << 10
)

type Store interface {
	ListMCPServers(context.Context) ([]mcpserver.Configuration, error)
	GetMCPServer(context.Context, string) (mcpserver.Configuration, error)
	SaveMCPServer(context.Context, mcpserver.Configuration, uint64) error
	DeleteMCPServer(context.Context, string, uint64) error
	PutMCPAuthorizationAttempt(context.Context, mcpserver.AuthorizationAttempt) error
	GetMCPAuthorizationAttempt(context.Context, string) (mcpserver.AuthorizationAttempt, error)
	RecoverMCPAuthorizationAttempts(context.Context, time.Time, time.Time) error
	PruneMCPAuthorizationAttempts(context.Context, time.Time) error
}

type IDs interface {
	New(string) (string, error)
}

type Events interface {
	Publish(protocol.RuntimeEvent)
}

type Config struct {
	Store    Store
	IDs      IDs
	Events   Events
	Lifetime context.Context
	Logger   *slog.Logger
}

type liveServer struct {
	generation uint64
	status     protocol.MCPServerState
	session    *sdkmcp.ClientSession
	cancel     context.CancelFunc
	tools      []protocol.MCPTool
}

type Service struct {
	store    Store
	ids      IDs
	events   Events
	lifetime context.Context
	openURL  func(context.Context, string) error
	lanes    *identitylane.Coordinator
	logger   *slog.Logger

	mu     sync.Mutex
	live   map[string]*liveServer
	tasks  sync.WaitGroup
	closed bool
}

func New(config Config) (*Service, error) {
	if config.Store == nil || config.IDs == nil || config.Events == nil || config.Lifetime == nil {
		return nil, errors.New("mcpflow: store, ids, events, and lifetime are required")
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	service := &Service{
		store: config.Store, ids: config.IDs, events: config.Events,
		lifetime: config.Lifetime,
		openURL: openSystemBrowser,
		lanes: identitylane.New(),
		logger: logger,
		live: make(map[string]*liveServer),
	}
	now := service.now()
	startupContext := context.WithoutCancel(config.Lifetime)
	if err := service.store.RecoverMCPAuthorizationAttempts(
		startupContext,
		now,
		now.Add(-time.Duration(protocol.DefaultMCPAttemptTTL)*time.Second),
	); err != nil {
		return nil, err
	}
	configurations, err := service.store.ListMCPServers(startupContext)
	if err != nil {
		return nil, err
	}
	for _, configuration := range configurations {
		if configuration.Enabled() {
			service.live[configuration.Name()] = &liveServer{
				status: protocol.MCPServerState{Type: protocol.MCPServerDisconnected},
			}
			service.connectAsync(configuration)
			continue
		}
		service.live[configuration.Name()] = &liveServer{
			status: protocol.MCPServerState{Type: protocol.MCPServerDisabled},
		}
	}
	return service, nil
}

func (service *Service) List(ctx context.Context) (*protocol.Page[protocol.MCPServer], error) {
	configurations, err := service.store.ListMCPServers(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]protocol.MCPServer, 0, len(configurations))
	for _, configuration := range configurations {
		values = append(values, service.present(configuration))
	}
	return protocol.NewPage(values), nil
}

func (service *Service) Create(
	ctx context.Context,
	candidate protocol.MCPServerCandidate,
) (*protocol.MCPServer, error) {
	patch, err := candidatePatch(candidate)
	if err != nil {
		return nil, err
	}
	configuration, err := mcpserver.New(candidate.Name, patch, service.now())
	if err != nil {
		return nil, invalidConfiguration(err)
	}
	release, err := service.lanes.Acquire(ctx, configuration.Name())
	if err != nil {
		return nil, err
	}
	defer release()
	if err := service.store.SaveMCPServer(ctx, configuration, 0); err != nil {
		return nil, projectStoreError(err)
	}
	service.activate(configuration)
	value := service.present(configuration)
	return &value, nil
}

func (service *Service) Update(
	ctx context.Context,
	request protocol.UpdateMCPServerRequest,
) (*protocol.MCPServer, error) {
	patch, reconnect, err := updatePatch(request)
	if err != nil {
		return nil, err
	}
	release, err := service.lanes.Acquire(ctx, request.Server)
	if err != nil {
		return nil, err
	}
	defer release()
	for range maximumPatchRetries {
		configuration, err := service.store.GetMCPServer(ctx, request.Server)
		if err != nil {
			return nil, projectStoreError(err)
		}
		previousRevision := configuration.Revision()
		previousEnabled := configuration.Enabled()
		changed, err := configuration.Apply(patch, service.now())
		if err != nil {
			return nil, invalidConfiguration(err)
		}
		if !changed {
			value := service.present(configuration)
			return &value, nil
		}
		if err := service.store.SaveMCPServer(ctx, configuration, previousRevision); err != nil {
			if errors.Is(err, mcpserver.ErrRevisionConflict) {
				continue
			}
			return nil, projectStoreError(err)
		}
		switch {
		case !configuration.Enabled():
			service.disconnect(configuration.Name(), protocol.MCPServerState{Type: protocol.MCPServerDisabled}, true)
		case reconnect || !previousEnabled:
			service.connectAsync(configuration)
		default:
			service.publish(configuration.Name())
		}
		value := service.present(configuration)
		return &value, nil
	}
	return nil, fmt.Errorf("mcpflow: server %q remained busy after concurrent updates", request.Server)
}

func (service *Service) Delete(ctx context.Context, name string) error {
	release, err := service.lanes.Acquire(ctx, name)
	if err != nil {
		return err
	}
	defer release()
	for range maximumPatchRetries {
		configuration, err := service.store.GetMCPServer(ctx, name)
		if err != nil {
			return projectStoreError(err)
		}
		if err := service.store.DeleteMCPServer(ctx, name, configuration.Revision()); err != nil {
			if errors.Is(err, mcpserver.ErrRevisionConflict) {
				continue
			}
			return projectStoreError(err)
		}
		service.removeLive(name)
		service.publish(name)
		return nil
	}
	return fmt.Errorf("mcpflow: server %q remained busy while deleting", name)
}

func (service *Service) Test(
	ctx context.Context,
	candidate protocol.MCPServerCandidate,
) (*protocol.MCPTestResult, error) {
	patch, err := candidatePatch(candidate)
	if err != nil {
		return nil, err
	}
	configuration, err := mcpserver.New(candidate.Name, patch, service.now())
	if err != nil {
		return nil, invalidConfiguration(err)
	}
	probeContext, cancelProbe := context.WithTimeout(ctx, connectionTimeout(configuration))
	defer cancelProbe()
	session, cancelSession, _, err := service.dial(probeContext, configuration, false)
	if cancelSession != nil {
		defer cancelSession()
	}
	if session != nil {
		defer session.Close()
	}
	if err != nil {
		return &protocol.MCPTestResult{Error: mcpProblem(err)}, nil
	}
	return &protocol.MCPTestResult{OK: true}, nil
}

func (service *Service) Tools(
	ctx context.Context,
	request protocol.MCPListToolsRequest,
) (*protocol.Page[protocol.MCPTool], error) {
	service.mu.Lock()
	values := make([]protocol.MCPTool, 0)
	for name, live := range service.live {
		if (request.Server == "" || request.Server == name) && live.status.Type == protocol.MCPServerConnected {
			for _, tool := range live.tools {
				values = append(values, cloneTool(tool))
			}
		}
	}
	service.mu.Unlock()
	if request.Server != "" {
		if _, err := service.store.GetMCPServer(ctx, request.Server); err != nil {
			return nil, projectStoreError(err)
		}
	}
	slices.SortFunc(values, func(left, right protocol.MCPTool) int {
		if left.Server != right.Server {
			return strings.Compare(left.Server, right.Server)
		}
		return strings.Compare(left.Name, right.Name)
	})
	return protocol.NewPage(values), nil
}

func (service *Service) Reconnect(ctx context.Context, name string) error {
	release, err := service.lanes.Acquire(ctx, name)
	if err != nil {
		return err
	}
	defer release()
	configuration, err := service.store.GetMCPServer(ctx, name)
	if err != nil {
		return projectStoreError(err)
	}
	if !configuration.Enabled() {
		return protocol.ErrMCPServerDisabled
	}
	service.connectAsync(configuration)
	return nil
}

func (service *Service) CreateAuthorizationAttempt(
	ctx context.Context,
	name string,
) (*protocol.MCPAuthorizationAttempt, error) {
	release, err := service.lanes.Acquire(ctx, name)
	if err != nil {
		return nil, err
	}
	defer release()
	configuration, err := service.store.GetMCPServer(ctx, name)
	if err != nil {
		return nil, projectStoreError(err)
	}
	if configuration.Transport() != mcpserver.TransportStreamableHTTP {
		return nil, fmt.Errorf("%w: OAuth requires streamable HTTP", protocol.ErrInvalidParams)
	}
	if !configuration.Enabled() {
		return nil, protocol.ErrMCPServerDisabled
	}
	id, err := service.ids.New("mcp_auth_")
	if err != nil {
		return nil, err
	}
	attempt, err := mcpserver.NewAuthorizationAttempt(id, name, service.now())
	if err != nil {
		return nil, err
	}
	if err := service.store.PruneMCPAuthorizationAttempts(ctx, service.attemptCutoff()); err != nil {
		return nil, err
	}
	if err := service.store.PutMCPAuthorizationAttempt(ctx, attempt); err != nil {
		return nil, err
	}
	if !service.startAuthorization(configuration, attempt) {
		_ = attempt.Finish(mcpserver.AuthorizationCanceled, service.now())
		_ = service.store.PutMCPAuthorizationAttempt(context.WithoutCancel(ctx), attempt)
		return nil, errors.New("mcpflow: service is closing")
	}
	value := presentAuthorizationAttempt(attempt)
	return &value, nil
}

func (service *Service) GetAuthorizationAttempt(
	ctx context.Context,
	id string,
) (*protocol.MCPAuthorizationAttempt, error) {
	if err := service.store.PruneMCPAuthorizationAttempts(ctx, service.attemptCutoff()); err != nil {
		return nil, err
	}
	value, err := service.store.GetMCPAuthorizationAttempt(ctx, id)
	if errors.Is(err, mcpserver.ErrNotFound) {
		return nil, protocol.ErrMCPAuthorizationAttemptNotFound
	}
	if err != nil {
		return nil, err
	}
	presented := presentAuthorizationAttempt(value)
	return &presented, nil
}

func (service *Service) Close() {
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return
	}
	service.closed = true
	sessions := make([]*sdkmcp.ClientSession, 0, len(service.live))
	for _, live := range service.live {
		live.generation++
		if live.cancel != nil {
			live.cancel()
		}
		if live.session != nil {
			sessions = append(sessions, live.session)
		}
	}
	service.mu.Unlock()
	for _, session := range sessions {
		_ = session.Close()
	}
	service.tasks.Wait()
}

func (service *Service) activate(configuration mcpserver.Configuration) {
	if configuration.Enabled() {
		service.connectAsync(configuration)
		return
	}
	service.disconnect(configuration.Name(), protocol.MCPServerState{Type: protocol.MCPServerDisabled}, true)
}

func (service *Service) connectAsync(configuration mcpserver.Configuration) {
	name := configuration.Name()
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return
	}
	live := service.live[name]
	if live == nil {
		live = &liveServer{}
		service.live[name] = live
	}
	live.generation++
	generation := live.generation
	if live.cancel != nil {
		live.cancel()
	}
	oldSession := live.session
	live.session = nil
	live.cancel = nil
	live.tools = nil
	live.status = protocol.MCPServerState{Type: protocol.MCPServerConnecting}
	service.tasks.Add(1)
	service.mu.Unlock()
	if oldSession != nil {
		_ = oldSession.Close()
	}
	service.publish(name)
	go func() {
		defer service.tasks.Done()
		connectContext, cancelConnect := context.WithTimeout(service.lifetime, connectionTimeout(configuration))
		defer cancelConnect()
		session, sessionCancel, tools, err := service.dial(connectContext, configuration, true)
		service.commitDial(name, generation, session, sessionCancel, tools, err)
	}()
}

func (service *Service) commitDial(
	name string,
	generation uint64,
	session *sdkmcp.ClientSession,
	cancel context.CancelFunc,
	tools []protocol.MCPTool,
	dialErr error,
) {
	service.mu.Lock()
	live := service.live[name]
	if service.closed || live == nil || live.generation != generation {
		service.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if session != nil {
			_ = session.Close()
		}
		return
	}
	watch := false
	if dialErr != nil {
		live.status = mcpProblemState(dialErr)
	} else {
		count := len(tools)
		live.status = protocol.MCPServerState{Type: protocol.MCPServerConnected, ToolCount: &count}
		live.session = session
		live.cancel = cancel
		live.tools = tools
		service.tasks.Add(1)
		watch = true
	}
	service.mu.Unlock()
	service.publish(name)
	if watch {
		go service.watchSession(name, generation, session)
	}
}

func (service *Service) watchSession(
	name string,
	generation uint64,
	session *sdkmcp.ClientSession,
) {
	defer service.tasks.Done()
	waitErr := session.Wait()
	service.mu.Lock()
	live := service.live[name]
	if service.closed || live == nil || live.generation != generation || live.session != session {
		service.mu.Unlock()
		return
	}
	live.session = nil
	live.cancel = nil
	live.tools = nil
	if waitErr != nil && !errors.Is(waitErr, context.Canceled) {
		live.status = mcpProblemState(waitErr)
	} else {
		live.status = protocol.MCPServerState{Type: protocol.MCPServerDisconnected}
	}
	service.mu.Unlock()
	service.publish(name)
}

func (service *Service) clientFor(name string, observeToolChanges bool) *sdkmcp.Client {
	var options *sdkmcp.ClientOptions
	if observeToolChanges {
		options = &sdkmcp.ClientOptions{
			ToolListChangedHandler: func(context.Context, *sdkmcp.ToolListChangedRequest) {
				service.startToolRefresh(name)
			},
		}
	}
	return sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "lyra-runtime", Version: "app2"},
		options,
	)
}

func (service *Service) startToolRefresh(name string) {
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return
	}
	service.tasks.Add(1)
	service.mu.Unlock()
	go func() {
		defer service.tasks.Done()
		ctx, cancel := context.WithTimeout(service.lifetime, 5*time.Second)
		defer cancel()
		configuration, err := service.store.GetMCPServer(ctx, name)
		if err != nil || !configuration.Enabled() {
			return
		}
		service.connectAsync(configuration)
	}()
}

func (service *Service) dial(
	ctx context.Context,
	configuration mcpserver.Configuration,
	observeToolChanges bool,
) (*sdkmcp.ClientSession, context.CancelFunc, []protocol.MCPTool, error) {
	sessionContext, cancel := context.WithCancel(service.lifetime)
	stop := context.AfterFunc(ctx, cancel)
	secrets := configuration.Secrets()
	var transport sdkmcp.Transport
	switch configuration.Transport() {
	case mcpserver.TransportStreamableHTTP:
		handler, err := service.passiveOAuthHandler(configuration)
		if err != nil && len(secrets.OAuthSession) > 0 {
			writer := newOAuthSessionWriter(service, configuration)
			if clearErr := writer.Clear(ctx); clearErr != nil {
				cancel()
				return nil, nil, nil, errors.Join(err, clearErr)
			}
			configuration = writer.Configuration()
			secrets = configuration.Secrets()
			handler, err = service.passiveOAuthHandler(configuration)
		}
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}
		client, err := secureHTTPClient(configuration.URL(), secrets, len(secrets.OAuthSession) == 0)
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}
		transport = &sdkmcp.StreamableClientTransport{
			Endpoint: configuration.URL(), HTTPClient: client, OAuthHandler: handler,
		}
	case mcpserver.TransportStdio:
		command := exec.CommandContext(sessionContext, configuration.Command(), configuration.Args()...)
		if len(secrets.Environment) > 0 {
			command.Env = append(os.Environ(), environment(secrets.Environment)...)
		}
		command.Dir = configuration.Dir()
		transport = &sdkmcp.CommandTransport{Command: command}
	default:
		cancel()
		return nil, nil, nil, errors.New("mcpflow: invalid transport")
	}
	session, err := service.clientFor(configuration.Name(), observeToolChanges).Connect(sessionContext, transport, nil)
	if err != nil {
		stop()
		cancel()
		return nil, nil, nil, err
	}
	if !stop() || ctx.Err() != nil {
		cancel()
		_ = session.Close()
		return nil, nil, nil, ctx.Err()
	}
	tools, err := listTools(ctx, configuration.Name(), configuration.DisabledTools(), session)
	if err != nil {
		cancel()
		_ = session.Close()
		return nil, nil, nil, err
	}
	return session, cancel, tools, nil
}

func listTools(
	ctx context.Context,
	server string,
	disabled []string,
	session *sdkmcp.ClientSession,
) ([]protocol.MCPTool, error) {
	disabledSet := make(map[string]struct{}, len(disabled))
	for _, name := range disabled {
		disabledSet[name] = struct{}{}
	}
	seen := make(map[string]bool)
	values := make([]protocol.MCPTool, 0)
	for descriptor, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		if _, err := mcpserver.ToolName(server, descriptor.Name); err != nil || seen[descriptor.Name] {
			return nil, errors.New("mcpflow: server returned an invalid or duplicate tool name")
		}
		if len(seen) >= maximumRemoteTools {
			return nil, errors.New("mcpflow: server returned too many tools")
		}
		if len(descriptor.Description) > maximumRemoteDescriptionBytes || !utf8.ValidString(descriptor.Description) {
			return nil, errors.New("mcpflow: server returned an invalid tool description")
		}
		seen[descriptor.Name] = true
		if _, omitted := disabledSet[descriptor.Name]; omitted {
			continue
		}
		encoded, err := json.Marshal(descriptor.InputSchema)
		if err != nil {
			return nil, err
		}
		if len(encoded) > maximumRemoteSchemaBytes {
			return nil, errors.New("mcpflow: server returned an oversized tool schema")
		}
		var schema map[string]any
		if err := json.Unmarshal(encoded, &schema); err != nil || schema == nil {
			if err == nil {
				err = errors.New("schema is not an object")
			}
			return nil, err
		}
		values = append(values, protocol.MCPTool{
			Server: server, Name: descriptor.Name,
			Description: descriptor.Description, InputSchema: schema,
		})
	}
	slices.SortFunc(values, func(left, right protocol.MCPTool) int {
		return strings.Compare(left.Name, right.Name)
	})
	return values, nil
}

func (service *Service) present(configuration mcpserver.Configuration) protocol.MCPServer {
	secrets := configuration.Secrets()
	connection := protocol.MCPConnection{
		Type: protocol.MCPTransport(configuration.Transport()),
		URL: configuration.URL(), Command: configuration.Command(),
		Args: configuration.Args(), Dir: configuration.Dir(),
	}
	if secrets.Authorization != "" || len(secrets.OAuthSession) > 0 {
		connection.AuthorizationMasked = "••••"
	}
	if len(secrets.Headers) > 0 {
		connection.HeadersMasked = make(map[string]string, len(secrets.Headers))
		for name := range secrets.Headers {
			connection.HeadersMasked[name] = "••••"
		}
	}
	if len(secrets.Environment) > 0 {
		connection.EnvMasked = make(map[string]string, len(secrets.Environment))
		for name := range secrets.Environment {
			connection.EnvMasked[name] = "••••"
		}
	}
	status := protocol.MCPServerState{Type: protocol.MCPServerDisconnected}
	service.mu.Lock()
	if live := service.live[configuration.Name()]; live != nil {
		status = live.status
	}
	service.mu.Unlock()
	if !configuration.Enabled() {
		status = protocol.MCPServerState{Type: protocol.MCPServerDisabled}
	}
	return protocol.MCPServer{
		Name: configuration.Name(), Description: configuration.Description(),
		Connection: connection, TimeoutSeconds: configuration.TimeoutSeconds(),
		DisabledTools: configuration.DisabledTools(),
		AutoApproveTools: configuration.AutoApproveTools(), Status: status,
	}
}

func candidatePatch(candidate protocol.MCPServerCandidate) (mcpserver.Patch, error) {
	connection, err := connectionPatch(candidate.Connection)
	if err != nil {
		return mcpserver.Patch{}, err
	}
	enabled := candidate.Enabled
	return mcpserver.Patch{
		Enabled: &enabled, Description: &candidate.Description,
		Connection: &connection, TimeoutSeconds: &candidate.TimeoutSeconds,
		DisabledTools: &candidate.DisabledTools,
		AutoApproveTools: &candidate.AutoApproveTools,
	}, nil
}

func updatePatch(request protocol.UpdateMCPServerRequest) (mcpserver.Patch, bool, error) {
	patch := mcpserver.Patch{
		Enabled: request.Enabled, Description: request.Description,
		TimeoutSeconds: request.TimeoutSeconds,
		DisabledTools: request.DisabledTools,
		AutoApproveTools: request.AutoApproveTools,
	}
	if request.Connection != nil {
		connection, err := connectionPatch(*request.Connection)
		if err != nil {
			return mcpserver.Patch{}, false, err
		}
		patch.Connection = &connection
	}
	reconnect := request.Connection != nil || request.TimeoutSeconds != nil || request.DisabledTools != nil
	return patch, reconnect, nil
}

func connectionPatch(input protocol.MCPConnectionInput) (mcpserver.ConnectionPatch, error) {
	connection := mcpserver.ConnectionPatch{
		Transport: mcpserver.Transport(input.Type), URL: input.URL,
		Command: input.Command, Args: slices.Clone(input.Args), Dir: input.Dir,
	}
	var err error
	if connection.Authorization, err = textSecretChange(input.Authorization); err != nil {
		return mcpserver.ConnectionPatch{}, err
	}
	if connection.Headers, err = mapSecretChange(input.Headers); err != nil {
		return mcpserver.ConnectionPatch{}, err
	}
	if connection.Environment, err = environmentSecretChange(input.Env); err != nil {
		return mcpserver.ConnectionPatch{}, err
	}
	return connection, nil
}

func textSecretChange(change *protocol.MCPAuthorizationChange) (mcpserver.SecretChange[string], error) {
	if change == nil {
		return mcpserver.SecretChange[string]{}, nil
	}
	switch change.Type {
	case protocol.MCPSecretSet:
		return mcpserver.SecretChange[string]{Set: true, Value: change.Value}, nil
	case protocol.MCPSecretClear:
		return mcpserver.SecretChange[string]{Clear: true}, nil
	default:
		return mcpserver.SecretChange[string]{}, fmt.Errorf("%w: invalid authorization change", protocol.ErrInvalidParams)
	}
}

func mapSecretChange(change *protocol.MCPHeadersChange) (mcpserver.SecretChange[map[string]string], error) {
	if change == nil {
		return mcpserver.SecretChange[map[string]string]{}, nil
	}
	switch change.Type {
	case protocol.MCPSecretSet:
		return mcpserver.SecretChange[map[string]string]{Set: true, Value: maps.Clone(change.Value)}, nil
	case protocol.MCPSecretClear:
		return mcpserver.SecretChange[map[string]string]{Clear: true}, nil
	default:
		return mcpserver.SecretChange[map[string]string]{}, fmt.Errorf("%w: invalid header change", protocol.ErrInvalidParams)
	}
}

func environmentSecretChange(change *protocol.MCPEnvironmentChange) (mcpserver.SecretChange[map[string]string], error) {
	if change == nil {
		return mcpserver.SecretChange[map[string]string]{}, nil
	}
	switch change.Type {
	case protocol.MCPSecretSet:
		return mcpserver.SecretChange[map[string]string]{Set: true, Value: maps.Clone(change.Value)}, nil
	case protocol.MCPSecretClear:
		return mcpserver.SecretChange[map[string]string]{Clear: true}, nil
	default:
		return mcpserver.SecretChange[map[string]string]{}, fmt.Errorf("%w: invalid environment change", protocol.ErrInvalidParams)
	}
}

func secureHTTPClient(
	endpoint string,
	secrets mcpserver.SecretState,
	explicitAuthorization bool,
) (*http.Client, error) {
	origin, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	base := http.DefaultTransport
	return &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			if !sameOrigin(origin, request.URL) {
				return nil, errors.New("mcpflow: cross-origin credential redirect refused")
			}
			clone := request.Clone(request.Context())
			clone.Header = request.Header.Clone()
			for name, value := range secrets.Headers {
				if explicitAuthorization || !strings.EqualFold(name, "Authorization") {
					clone.Header.Set(name, value)
				}
			}
			if explicitAuthorization && secrets.Authorization != "" {
				clone.Header.Set("Authorization", secrets.Authorization)
			}
			return base.RoundTrip(clone)
		}),
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 || !sameOrigin(origin, request.URL) {
				return errors.New("mcpflow: redirect refused")
			}
			return nil
		},
	}, nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func (service *Service) disconnect(name string, status protocol.MCPServerState, publish bool) {
	service.mu.Lock()
	live := service.live[name]
	if live == nil {
		live = &liveServer{}
		service.live[name] = live
	}
	live.generation++
	if live.cancel != nil {
		live.cancel()
	}
	session := live.session
	live.cancel = nil
	live.session = nil
	live.tools = nil
	live.status = status
	service.mu.Unlock()
	if session != nil {
		_ = session.Close()
	}
	if publish {
		service.publish(name)
	}
}

func (service *Service) removeLive(name string) {
	service.mu.Lock()
	live := service.live[name]
	if live != nil {
		live.generation++
		if live.cancel != nil {
			live.cancel()
		}
	}
	var session *sdkmcp.ClientSession
	if live != nil {
		session = live.session
	}
	delete(service.live, name)
	service.mu.Unlock()
	if session != nil {
		_ = session.Close()
	}
}

func (service *Service) publish(name string) {
	service.events.Publish(protocol.RuntimeEvent{
		Type: protocol.RuntimeMCPChanged, ServerIDs: []string{name},
	})
}

func projectStoreError(err error) error {
	switch {
	case errors.Is(err, mcpserver.ErrNotFound):
		return protocol.ErrMCPServerNotFound
	case errors.Is(err, mcpserver.ErrExists):
		return protocol.ErrMCPServerAlreadyExists
	default:
		return err
	}
}

func invalidConfiguration(err error) error {
	return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
}

func mcpProblem(err error) *protocol.ProblemData {
	if errors.Is(err, errOAuthRequired) {
		return &protocol.ProblemData{
			Type: protocol.ProblemMCPAuthorizationRequired,
		}
	}
	return &protocol.ProblemData{Type: protocol.ProblemMCPDialFailed}
}

func mcpProblemState(err error) protocol.MCPServerState {
	problem := mcpProblem(err)
	if errors.Is(err, errOAuthRequired) {
		return protocol.MCPServerState{Type: protocol.MCPServerNeedsAuth, Error: problem}
	}
	return protocol.MCPServerState{Type: protocol.MCPServerFailed, Error: problem}
}

func presentAuthorizationAttempt(value mcpserver.AuthorizationAttempt) protocol.MCPAuthorizationAttempt {
	status := protocol.MCPAuthorizationAttemptStatus{
		Type: protocol.MCPAuthorizationAttemptStatusType(value.Status()),
	}
	if value.Status() == mcpserver.AuthorizationFailed {
		status.Error = &protocol.ProblemData{Type: protocol.ProblemMCPAuthorizationFailed}
	}
	return protocol.MCPAuthorizationAttempt{
		ID: value.ID(), Server: value.Server(), Status: status,
		CreatedAt: value.CreatedAt(), FinishedAt: value.FinishedAt(),
	}
}

func connectionTimeout(configuration mcpserver.Configuration) time.Duration {
	if configuration.TimeoutSeconds() > 0 {
		return time.Duration(configuration.TimeoutSeconds()) * time.Second
	}
	return defaultTimeout
}

func cloneTool(value protocol.MCPTool) protocol.MCPTool {
	encoded, err := json.Marshal(value.InputSchema)
	if err != nil {
		return protocol.MCPTool{
			Server: value.Server, Name: value.Name, Description: value.Description,
		}
	}
	var schema map[string]any
	if json.Unmarshal(encoded, &schema) != nil {
		schema = nil
	}
	value.InputSchema = schema
	return value
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

func environment(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	slices.Sort(out)
	return out
}

func (service *Service) now() time.Time { return time.Now().UTC() }

func (service *Service) attemptCutoff() time.Time {
	return service.now().Add(-time.Duration(protocol.DefaultMCPAttemptTTL) * time.Second)
}

// Call exposes a connected raw MCP tool to the Agent orchestration layer. The
// lossless server/name pair is never reconstructed from the model-visible name.
func (service *Service) Call(
	ctx context.Context,
	server string,
	name string,
	arguments map[string]any,
) (*sdkmcp.CallToolResult, error) {
	service.mu.Lock()
	live := service.live[server]
	var session *sdkmcp.ClientSession
	var status protocol.MCPServerStateType
	if live != nil {
		status = live.status.Type
		if status == protocol.MCPServerConnected {
			session = live.session
		}
	}
	service.mu.Unlock()
	if session == nil {
		return nil, fmt.Errorf("%w: server %q is %s", mcpserver.ErrUnavailable, server, status)
	}
	return session.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: arguments})
}

// CallText preserves the complete MCP result envelope, including structured
// content, resource links, annotations, and the remote error bit.
func (service *Service) CallText(
	ctx context.Context,
	server string,
	name string,
	arguments map[string]any,
) (string, error) {
	result, err := service.Call(ctx, server, name, arguments)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("mcpflow: encode tool result: %w", err)
	}
	return string(encoded), nil
}

func (service *Service) AutoApprovedTools(ctx context.Context, server string) ([]string, error) {
	configuration, err := service.store.GetMCPServer(ctx, server)
	if err != nil {
		return nil, projectStoreError(err)
	}
	return configuration.AutoApproveTools(), nil
}
