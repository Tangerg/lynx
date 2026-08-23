// Package mcpflow owns durable MCP configuration and generation-safe live
// transport sessions. Secrets cross only the write/store/dial boundary and are
// never projected back to callers.
package mcpflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app2/runtime/domain/mcpserver"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const defaultTimeout = 15 * time.Second

type Store interface {
	ListMCPServerRecords(context.Context) ([]mcpserver.Record, error)
	GetMCPServerRecord(context.Context, string) (mcpserver.Record, mcpserver.Secrets, error)
	CreateMCPServerRecord(context.Context, mcpserver.Record, mcpserver.Secrets) error
	PutMCPServerRecord(context.Context, mcpserver.Record, mcpserver.Secrets) error
	DeleteMCPServerRecord(context.Context, string) error
	PutMCPOAuthSession(context.Context, string, string, []byte) error
	ClearMCPOAuthSession(context.Context, string) error
	PutMCPAuthorizationAttempt(context.Context, protocol.MCPAuthorizationAttempt) error
	GetMCPAuthorizationAttempt(context.Context, string) (protocol.MCPAuthorizationAttempt, error)
}

type IDs interface{ New(string) (string, error) }

type liveServer struct {
	generation uint64
	status protocol.MCPServerState
	session *sdkmcp.ClientSession
	cancel context.CancelFunc
	tools []protocol.MCPTool
}

type Service struct {
	store Store
	ids IDs
	lifetime context.Context
	client *sdkmcp.Client
	openURL func(context.Context, string) error

	mu sync.Mutex
	live map[string]*liveServer
	tasks sync.WaitGroup
	closed bool
}

func New(store Store, ids IDs, lifetime context.Context) (*Service, error) {
	if store == nil || ids == nil || lifetime == nil {
		return nil, errors.New("mcpflow: store, ids and lifetime are required")
	}
	service := &Service{
		store: store, ids: ids, lifetime: lifetime,
		client: sdkmcp.NewClient(&sdkmcp.Implementation{Name: "lyra-runtime", Version: "app2"}, nil),
		openURL: openSystemBrowser,
		live: make(map[string]*liveServer),
	}
	records, err := store.ListMCPServerRecords(context.WithoutCancel(lifetime))
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if !record.Enabled {
			service.live[record.Name] = &liveServer{status: protocol.MCPServerState{Type: protocol.MCPServerDisabled}}
			continue
		}
		service.live[record.Name] = &liveServer{status: protocol.MCPServerState{Type: protocol.MCPServerDisconnected}}
		service.connectAsync(record.Name)
	}
	return service, nil
}

func (service *Service) List(ctx context.Context) (*protocol.Page[protocol.MCPServer], error) {
	records, err := service.store.ListMCPServerRecords(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]protocol.MCPServer, 0, len(records))
	for _, record := range records {
		values = append(values, service.present(record))
	}
	return protocol.NewPage(values), nil
}

func (service *Service) Create(ctx context.Context, candidate protocol.MCPServerCandidate) (*protocol.MCPServer, error) {
	record, secrets, err := candidateRecord(candidate, service.now())
	if err != nil {
		return nil, err
	}
	if _, _, lookupErr := service.store.GetMCPServerRecord(ctx, record.Name); lookupErr == nil {
		return nil, protocol.ErrMCPServerAlreadyExists
	} else if !errors.Is(lookupErr, mcpserver.ErrNotFound) {
		return nil, lookupErr
	}
	if err := service.store.CreateMCPServerRecord(ctx, record, secrets); err != nil {
		if _, _, lookupErr := service.store.GetMCPServerRecord(ctx, record.Name); lookupErr == nil {
			return nil, protocol.ErrMCPServerAlreadyExists
		}
		return nil, err
	}
	if record.Enabled {
		service.connectAsync(record.Name)
	} else {
		service.setDisabled(record.Name)
	}
	value := service.present(record)
	return &value, nil
}

func (service *Service) Update(ctx context.Context, request protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error) {
	record, secrets, err := service.store.GetMCPServerRecord(ctx, request.Server)
	if err != nil {
		return nil, projectStoreError(err)
	}
	if request.Enabled != nil {
		record.Enabled = *request.Enabled
	}
	if request.Description != nil {
		record.Description = *request.Description
	}
	if request.TimeoutSeconds != nil {
		record.TimeoutSeconds = *request.TimeoutSeconds
	}
	if request.DisabledTools != nil {
		record.DisabledTools = slices.Clone(*request.DisabledTools)
	}
	if request.AutoApproveTools != nil {
		record.AutoApproveTools = slices.Clone(*request.AutoApproveTools)
	}
	if request.Connection != nil {
		if record.Transport != request.Connection.Type {
			secrets = mcpserver.Secrets{}
		}
		if err := applyConnection(&record, &secrets, *request.Connection); err != nil {
			return nil, err
		}
	}
	record.UpdatedAt = service.now()
	if err := validateRecord(record, secrets); err != nil {
		return nil, err
	}
	if err := service.store.PutMCPServerRecord(ctx, record, secrets); err != nil {
		return nil, projectStoreError(err)
	}
	if record.Enabled {
		service.connectAsync(record.Name)
	} else {
		service.disconnect(record.Name, protocol.MCPServerState{Type: protocol.MCPServerDisabled})
	}
	value := service.present(record)
	return &value, nil
}

func (service *Service) Delete(ctx context.Context, name string) error {
	if err := service.store.DeleteMCPServerRecord(ctx, name); err != nil {
		return projectStoreError(err)
	}
	service.disconnect(name, protocol.MCPServerState{Type: protocol.MCPServerDisconnected})
	service.mu.Lock()
	delete(service.live, name)
	service.mu.Unlock()
	return nil
}

func (service *Service) Test(ctx context.Context, candidate protocol.MCPServerCandidate) (*protocol.MCPTestResult, error) {
	record, secrets, err := candidateRecord(candidate, service.now())
	if err != nil {
		return nil, err
	}
	session, cancel, _, err := service.dial(ctx, record, secrets)
	if cancel != nil {
		defer cancel()
	}
	if session != nil {
		defer session.Close()
	}
	if err != nil {
		return &protocol.MCPTestResult{OK: false, Error: mcpProblem(err)}, nil
	}
	return &protocol.MCPTestResult{OK: true}, nil
}

func (service *Service) Tools(ctx context.Context, request protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error) {
	service.mu.Lock()
	values := make([]protocol.MCPTool, 0)
	for name, live := range service.live {
		if (request.Server == "" || request.Server == name) && live.status.Type == protocol.MCPServerConnected {
			values = append(values, live.tools...)
		}
	}
	service.mu.Unlock()
	if request.Server != "" {
		if _, _, err := service.store.GetMCPServerRecord(ctx, request.Server); err != nil {
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
	record, _, err := service.store.GetMCPServerRecord(ctx, name)
	if err != nil {
		return projectStoreError(err)
	}
	if !record.Enabled {
		return protocol.ErrMCPServerDisabled
	}
	service.connectAsync(name)
	return nil
}

func (service *Service) CreateAuthorizationAttempt(ctx context.Context, name string) (*protocol.MCPAuthorizationAttempt, error) {
	record, secrets, err := service.store.GetMCPServerRecord(ctx, name)
	if err != nil {
		return nil, projectStoreError(err)
	}
	if record.Transport != protocol.MCPTransportStreamableHTTP {
		return nil, fmt.Errorf("%w: OAuth requires streamable HTTP", protocol.ErrInvalidParams)
	}
	if !record.Enabled {
		return nil, protocol.ErrMCPServerDisabled
	}
	id, err := service.ids.New("mcp_auth_")
	if err != nil {
		return nil, err
	}
	now := service.now()
	value := protocol.MCPAuthorizationAttempt{
		ID: id, Server: name, Status: protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptPending}, CreatedAt: now,
	}
	if err := service.store.PutMCPAuthorizationAttempt(ctx, value); err != nil {
		return nil, err
	}
	if !service.startAuthorization(record, secrets, value) {
		now := service.now()
		value.FinishedAt = &now
		value.Status = protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptCanceled}
		_ = service.store.PutMCPAuthorizationAttempt(context.WithoutCancel(ctx), value)
		return nil, errors.New("mcpflow: service is closing")
	}
	return &value, nil
}

func (service *Service) GetAuthorizationAttempt(ctx context.Context, id string) (*protocol.MCPAuthorizationAttempt, error) {
	value, err := service.store.GetMCPAuthorizationAttempt(ctx, id)
	if errors.Is(err, mcpserver.ErrNotFound) {
		return nil, protocol.ErrMCPAuthorizationAttemptNotFound
	}
	return &value, err
}

func (service *Service) Close() {
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return
	}
	service.closed = true
	var sessions []*sdkmcp.ClientSession
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
	service.tasks.Wait()
	for _, session := range sessions {
		_ = session.Close()
	}
}

func (service *Service) connectAsync(name string) {
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
	old := live.session
	live.session = nil
	live.cancel = nil
	live.tools = nil
	live.status = protocol.MCPServerState{Type: protocol.MCPServerConnecting}
	service.tasks.Add(1)
	service.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	go func() {
		defer service.tasks.Done()
		ctx, cancel := context.WithTimeout(service.lifetime, defaultTimeout)
		defer cancel()
		record, secrets, err := service.store.GetMCPServerRecord(ctx, name)
		if err != nil {
			service.commitDial(name, generation, nil, nil, nil, err)
			return
		}
		session, sessionCancel, tools, err := service.dial(ctx, record, secrets)
		service.commitDial(name, generation, session, sessionCancel, tools, err)
	}()
}

func (service *Service) commitDial(name string, generation uint64, session *sdkmcp.ClientSession, cancel context.CancelFunc, tools []protocol.MCPTool, dialErr error) {
	service.mu.Lock()
	live := service.live[name]
	if service.closed || live == nil || live.generation != generation {
		service.mu.Unlock()
		if cancel != nil { cancel() }
		if session != nil { _ = session.Close() }
		return
	}
	if dialErr != nil {
		live.status = *mcpProblemState(dialErr)
	} else {
		count := len(tools)
		live.status = protocol.MCPServerState{Type: protocol.MCPServerConnected, ToolCount: &count}
		live.session, live.cancel, live.tools = session, cancel, tools
	}
	service.mu.Unlock()
}

func (service *Service) dial(ctx context.Context, record mcpserver.Record, secrets mcpserver.Secrets) (*sdkmcp.ClientSession, context.CancelFunc, []protocol.MCPTool, error) {
	sessionContext, cancel := context.WithCancel(service.lifetime)
	stop := context.AfterFunc(ctx, cancel)
	var transport sdkmcp.Transport
	switch record.Transport {
	case protocol.MCPTransportStreamableHTTP:
		origin, err := endpointOrigin(record.URL)
		if err != nil { cancel(); return nil, nil, nil, err }
		payload := secrets.OAuthSession
		if len(payload) > 0 && secrets.OAuthOrigin != origin {
			if clearErr := service.store.ClearMCPOAuthSession(ctx, record.Name); clearErr != nil {
				cancel(); return nil, nil, nil, clearErr
			}
			payload = nil
		}
		handler, err := service.passiveOAuthHandler(record.Name, record.URL, payload)
		if err != nil && len(payload) > 0 {
			if clearErr := service.store.ClearMCPOAuthSession(ctx, record.Name); clearErr != nil {
				cancel(); return nil, nil, nil, errors.Join(err, clearErr)
			}
			payload = nil
			handler, err = service.passiveOAuthHandler(record.Name, record.URL, nil)
		}
		if err != nil { cancel(); return nil, nil, nil, err }
		client, err := secureHTTPClient(record.URL, secrets, len(payload) == 0)
		if err != nil { cancel(); return nil, nil, nil, err }
		transport = &sdkmcp.StreamableClientTransport{Endpoint: record.URL, HTTPClient: client, OAuthHandler: handler}
	case protocol.MCPTransportStdio:
		command := exec.CommandContext(sessionContext, record.Command, record.Args...)
		if len(secrets.Environment) > 0 {
			command.Env = append(os.Environ(), environment(secrets.Environment)...)
		}
		command.Dir = record.Dir
		transport = &sdkmcp.CommandTransport{Command: command}
	default:
		cancel(); return nil, nil, nil, errors.New("mcpflow: invalid transport")
	}
	session, err := service.client.Connect(sessionContext, transport, nil)
	if err != nil {
		stop(); cancel(); return nil, nil, nil, err
	}
	if !stop() || ctx.Err() != nil {
		cancel(); _ = session.Close(); return nil, nil, nil, ctx.Err()
	}
	tools, err := listTools(ctx, record.Name, record.DisabledTools, session)
	if err != nil {
		cancel(); _ = session.Close(); return nil, nil, nil, err
	}
	return session, cancel, tools, nil
}

func listTools(ctx context.Context, server string, disabled []string, session *sdkmcp.ClientSession) ([]protocol.MCPTool, error) {
	disabledSet := make(map[string]struct{}, len(disabled))
	for _, name := range disabled {
		disabledSet[name] = struct{}{}
	}
	values := make([]protocol.MCPTool, 0)
	for descriptor, err := range session.Tools(ctx, nil) {
		if err != nil { return nil, err }
		if _, omitted := disabledSet[descriptor.Name]; omitted {
			continue
		}
		encoded, err := json.Marshal(descriptor.InputSchema)
		if err != nil { return nil, err }
		var schema map[string]any
		if err := json.Unmarshal(encoded, &schema); err != nil { return nil, err }
		values = append(values, protocol.MCPTool{Server: server, Name: descriptor.Name, Description: descriptor.Description, InputSchema: schema})
	}
	return values, nil
}

func (service *Service) present(record mcpserver.Record) protocol.MCPServer {
	connection := protocol.MCPConnection{Type: record.Transport, URL: record.URL, Command: record.Command, Args: slices.Clone(record.Args), Dir: record.Dir}
	if record.AuthorizationSet || record.OAuthSet { connection.AuthorizationMasked = "••••" }
	if len(record.HeaderNames) > 0 {
		connection.HeadersMasked = make(map[string]string, len(record.HeaderNames))
		for _, name := range record.HeaderNames { connection.HeadersMasked[name] = "••••" }
	}
	if len(record.EnvironmentNames) > 0 {
		connection.EnvMasked = make(map[string]string, len(record.EnvironmentNames))
		for _, name := range record.EnvironmentNames { connection.EnvMasked[name] = "••••" }
	}
	status := protocol.MCPServerState{Type: protocol.MCPServerDisconnected}
	service.mu.Lock()
	if live := service.live[record.Name]; live != nil { status = live.status }
	service.mu.Unlock()
	if !record.Enabled { status = protocol.MCPServerState{Type: protocol.MCPServerDisabled} }
	return protocol.MCPServer{Name: record.Name, Description: record.Description, Connection: connection, TimeoutSeconds: record.TimeoutSeconds, DisabledTools: slices.Clone(record.DisabledTools), AutoApproveTools: slices.Clone(record.AutoApproveTools), Status: status}
}

func candidateRecord(candidate protocol.MCPServerCandidate, now time.Time) (mcpserver.Record, mcpserver.Secrets, error) {
	record := mcpserver.Record{Name: strings.TrimSpace(candidate.Name), Enabled: candidate.Enabled, Description: candidate.Description, TimeoutSeconds: candidate.TimeoutSeconds, DisabledTools: slices.Clone(candidate.DisabledTools), AutoApproveTools: slices.Clone(candidate.AutoApproveTools), UpdatedAt: now}
	secrets := mcpserver.Secrets{}
	if err := applyConnection(&record, &secrets, candidate.Connection); err != nil { return mcpserver.Record{}, mcpserver.Secrets{}, err }
	if err := validateRecord(record, secrets); err != nil { return mcpserver.Record{}, mcpserver.Secrets{}, err }
	return record, secrets, nil
}

func applyConnection(record *mcpserver.Record, secrets *mcpserver.Secrets, input protocol.MCPConnectionInput) error {
	previousTransport, previousURL := record.Transport, record.URL
	record.Transport, record.URL, record.Command, record.Args, record.Dir = input.Type, input.URL, input.Command, slices.Clone(input.Args), input.Dir
	if input.Authorization != nil {
		switch input.Authorization.Type { case protocol.MCPSecretSet: secrets.Authorization = input.Authorization.Value; case protocol.MCPSecretClear: secrets.Authorization = ""; default: return fmt.Errorf("%w: invalid authorization change", protocol.ErrInvalidParams) }
	}
	if input.Headers != nil {
		switch input.Headers.Type { case protocol.MCPSecretSet: secrets.Headers = maps.Clone(input.Headers.Value); case protocol.MCPSecretClear: secrets.Headers = nil; default: return fmt.Errorf("%w: invalid headers change", protocol.ErrInvalidParams) }
	}
	if input.Env != nil {
		switch input.Env.Type { case protocol.MCPSecretSet: secrets.Environment = maps.Clone(input.Env.Value); case protocol.MCPSecretClear: secrets.Environment = nil; default: return fmt.Errorf("%w: invalid environment change", protocol.ErrInvalidParams) }
	}
	clearOAuth := input.Authorization != nil || hasAuthorizationHeader(input.Headers)
	if previousTransport != "" && (previousTransport != record.Transport || !sameEndpointOrigin(previousURL, record.URL)) {
		clearOAuth = true
	}
	if clearOAuth {
		secrets.OAuthOrigin = ""
		secrets.OAuthSession = nil
	}
	record.AuthorizationSet = secrets.Authorization != ""
	record.OAuthSet = len(secrets.OAuthSession) > 0
	record.HeaderNames = sortedKeys(secrets.Headers)
	record.EnvironmentNames = sortedKeys(secrets.Environment)
	return nil
}

func validateRecord(record mcpserver.Record, secrets mcpserver.Secrets) error {
	if record.Name == "" || strings.TrimSpace(record.Name) != record.Name || record.TimeoutSeconds < 0 {
		return fmt.Errorf("%w: invalid MCP server identity or timeout", protocol.ErrInvalidParams)
	}
	switch record.Transport {
	case protocol.MCPTransportStreamableHTTP:
		parsed, err := url.Parse(record.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || record.Command != "" || len(secrets.Environment) != 0 {
			return fmt.Errorf("%w: invalid streamable HTTP connection", protocol.ErrInvalidParams)
		}
	case protocol.MCPTransportStdio:
		if strings.TrimSpace(record.Command) == "" || record.URL != "" || secrets.Authorization != "" || len(secrets.Headers) != 0 || len(secrets.OAuthSession) != 0 {
			return fmt.Errorf("%w: invalid stdio connection", protocol.ErrInvalidParams)
		}
	default:
		return fmt.Errorf("%w: unknown MCP transport", protocol.ErrInvalidParams)
	}
	return nil
}

func secureHTTPClient(endpoint string, secrets mcpserver.Secrets, explicitAuthorization bool) (*http.Client, error) {
	origin, err := url.Parse(endpoint)
	if err != nil { return nil, err }
	base := http.DefaultTransport
	return &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if !sameOrigin(origin, request.URL) { return nil, errors.New("mcpflow: cross-origin credential redirect refused") }
		clone := request.Clone(request.Context())
		clone.Header = request.Header.Clone()
		for name, value := range secrets.Headers {
			if explicitAuthorization || !strings.EqualFold(name, "Authorization") { clone.Header.Set(name, value) }
		}
		if explicitAuthorization && secrets.Authorization != "" { clone.Header.Set("Authorization", secrets.Authorization) }
		return base.RoundTrip(clone)
	}), CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !sameOrigin(origin, request.URL) { return errors.New("mcpflow: redirect refused") }
		return nil
	}}, nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)
func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return function(request) }

func sameOrigin(left, right *url.URL) bool { return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host) }
func environment(values map[string]string) []string { out := make([]string, 0, len(values)); for key, value := range values { out = append(out, key+"="+value) }; slices.Sort(out); return out }
func sortedKeys[V any](values map[string]V) []string { out := slices.Collect(maps.Keys(values)); slices.Sort(out); return out }
func (service *Service) now() time.Time { return time.Now().UTC() }

func (service *Service) setDisabled(name string) { service.disconnect(name, protocol.MCPServerState{Type: protocol.MCPServerDisabled}) }
func (service *Service) disconnect(name string, status protocol.MCPServerState) {
	service.mu.Lock(); live := service.live[name]; if live == nil { live=&liveServer{}; service.live[name]=live }; live.generation++; if live.cancel != nil { live.cancel() }; session:=live.session; live.cancel=nil; live.session=nil; live.tools=nil; live.status=status; service.mu.Unlock(); if session != nil { _ = session.Close() }
}

func projectStoreError(err error) error { if errors.Is(err, mcpserver.ErrNotFound) { return protocol.ErrMCPServerNotFound }; if errors.Is(err, mcpserver.ErrExists) { return protocol.ErrMCPServerAlreadyExists }; return err }
func mcpProblem(err error) *protocol.ProblemData {
	if errors.Is(err, errOAuthRequired) {
		return &protocol.ProblemData{Type: protocol.ProblemMCPAuthorizationRequired, Detail: "the MCP server requires authorization"}
	}
	return &protocol.ProblemData{Type: protocol.ProblemMCPDialFailed, Detail: "the MCP server could not be reached"}
}
func mcpProblemState(err error) *protocol.MCPServerState {
	if errors.Is(err, errOAuthRequired) {
		return &protocol.MCPServerState{Type: protocol.MCPServerNeedsAuth, Error: mcpProblem(err)}
	}
	return &protocol.MCPServerState{Type: protocol.MCPServerFailed, Error: mcpProblem(err)}
}

func hasAuthorizationHeader(change *protocol.MCPHeadersChange) bool {
	if change == nil || change.Type != protocol.MCPSecretSet {
		return false
	}
	for name := range change.Value {
		if strings.EqualFold(name, "Authorization") { return true }
	}
	return false
}

func sameEndpointOrigin(left, right string) bool {
	leftOrigin, leftErr := endpointOrigin(left)
	rightOrigin, rightErr := endpointOrigin(right)
	return leftErr == nil && rightErr == nil && leftOrigin == rightOrigin
}

// Call exposes a connected raw MCP tool to the Agent/tool orchestration layer;
// callers identify it by the lossless (server, raw tool name) pair.
func (service *Service) Call(ctx context.Context, server, name string, arguments map[string]any) (*sdkmcp.CallToolResult, error) {
	service.mu.Lock(); live:=service.live[server]; var session *sdkmcp.ClientSession; if live!=nil && live.status.Type==protocol.MCPServerConnected { session=live.session }; service.mu.Unlock()
	if session==nil { return nil, protocol.ErrMCPServerDisabled }
	return session.CallTool(ctx, &sdkmcp.CallToolParams{Name:name, Arguments:arguments})
}

// CallText invokes one losslessly addressed MCP tool and serializes the full
// MCP result. Keeping the raw result envelope preserves structured content,
// resource links, annotations and the remote error bit for the model.
func (service *Service) CallText(ctx context.Context, server, name string, arguments map[string]any) (string, error) {
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

// AutoApprovedTools returns the remote names that bypass Lyra's ordinary
// network approval gate for one configured server. Names remain scoped by the
// server and are never compared with the collapsed model-visible name.
func (service *Service) AutoApprovedTools(ctx context.Context, server string) ([]string, error) {
	record, _, err := service.store.GetMCPServerRecord(ctx, server)
	if err != nil {
		return nil, projectStoreError(err)
	}
	return slices.Clone(record.AutoApproveTools), nil
}
