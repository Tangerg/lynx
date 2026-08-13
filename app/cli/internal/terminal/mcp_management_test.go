package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/mcp"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
)

type mcpServiceStub struct {
	mu          sync.Mutex
	servers     []mcp.Server
	created     chan mcp.Candidate
	probed      chan mcp.Candidate
	updated     chan mcp.ServerUpdate
	deleted     chan string
	reconnected chan string
	authReads   atomic.Int32
	authErrors  chan error
	now         time.Time
}

type blockingMCPAuthorizationService struct {
	*mcpServiceStub
	started  chan struct{}
	release  chan struct{}
	canceled chan struct{}
}

func (service *blockingMCPAuthorizationService) GetAuthorization(
	ctx context.Context,
	reference mcp.AuthorizationReference,
) (mcp.AuthorizationAttempt, error) {
	select {
	case service.started <- struct{}{}:
	default:
	}
	select {
	case <-service.release:
		return service.mcpServiceStub.GetAuthorization(ctx, reference)
	case <-ctx.Done():
		select {
		case service.canceled <- struct{}{}:
		default:
		}
		return mcp.AuthorizationAttempt{}, context.Cause(ctx)
	}
}

func newMCPServiceStub() *mcpServiceStub {
	count := 1
	return &mcpServiceStub{
		servers: []mcp.Server{{
			Name: "docs", Description: "Documentation", TimeoutSeconds: 15,
			Connection: mcp.Connection{Transport: mcp.StreamableHTTP, URL: "https://mcp.example/tools", AuthorizationMasked: "Bearer ****"},
			State:      mcp.State{Type: mcp.Connected, ToolCount: &count},
		}},
		created: make(chan mcp.Candidate, 1), probed: make(chan mcp.Candidate, 1),
		updated: make(chan mcp.ServerUpdate, 1),
		deleted: make(chan string, 1), reconnected: make(chan string, 1), now: time.Unix(100, 0),
	}
}

func (service *mcpServiceStub) Servers(context.Context) ([]mcp.Server, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	servers := make([]mcp.Server, len(service.servers))
	for index := range service.servers {
		servers[index] = service.servers[index].Clone()
	}
	return servers, nil
}

func (service *mcpServiceStub) CreateServer(_ context.Context, candidate mcp.Candidate) (mcp.Server, error) {
	if err := candidate.Validate(); err != nil {
		return mcp.Server{}, err
	}
	service.created <- candidate.Clone()
	server := mcp.Server{
		Name: candidate.Name, Description: candidate.Description, TimeoutSeconds: candidate.TimeoutSeconds,
		Connection:    mcp.Connection{Transport: candidate.Connection.Transport, URL: candidate.Connection.URL, Command: candidate.Connection.Command, Args: candidate.Connection.Args, Directory: candidate.Connection.Directory},
		DisabledTools: candidate.DisabledTools, AutoApproveTools: candidate.AutoApproveTools,
		State: mcp.State{Type: mcp.Disconnected},
	}
	service.mu.Lock()
	service.servers = append(service.servers, server)
	service.mu.Unlock()
	return server.Clone(), nil
}

func (service *mcpServiceStub) UpdateServer(_ context.Context, update mcp.ServerUpdate) (mcp.Server, error) {
	if err := update.Validate(); err != nil {
		return mcp.Server{}, err
	}
	service.updated <- update
	service.mu.Lock()
	defer service.mu.Unlock()
	for index := range service.servers {
		server := &service.servers[index]
		if server.Name != update.Server {
			continue
		}
		if update.Enabled != nil {
			if *update.Enabled {
				server.State = mcp.State{Type: mcp.Disconnected}
			} else {
				server.State = mcp.State{Type: mcp.Disabled}
			}
		}
		return server.Clone(), nil
	}
	return mcp.Server{}, errors.New("server not found")
}

func (service *mcpServiceStub) DeleteServer(_ context.Context, name string) error {
	service.deleted <- name
	service.mu.Lock()
	defer service.mu.Unlock()
	for index := range service.servers {
		if service.servers[index].Name == name {
			service.servers = append(service.servers[:index], service.servers[index+1:]...)
			return nil
		}
	}
	return errors.New("server not found")
}

func (service *mcpServiceStub) TestServer(_ context.Context, candidate mcp.Candidate) (mcp.TestResult, error) {
	if err := candidate.Validate(); err != nil {
		return mcp.TestResult{}, err
	}
	service.probed <- candidate.Clone()
	return mcp.TestResult{OK: true}, nil
}

func (*mcpServiceStub) Tools(_ context.Context, server string) ([]mcp.Tool, error) {
	if server != "" && server != "docs" {
		return nil, errors.New("server not found")
	}
	return []mcp.Tool{{Server: "docs", Name: "search", Description: "Search docs", InputSchema: []byte(`{"type":"object"}`)}}, nil
}

func (service *mcpServiceStub) ReconnectServer(_ context.Context, server string) error {
	service.reconnected <- server
	return nil
}

func (service *mcpServiceStub) StartAuthorization(_ context.Context, server string) (mcp.AuthorizationAttempt, error) {
	return mcp.AuthorizationAttempt{ID: "auth_1", Server: server, Status: mcp.AuthorizationPending, CreatedAt: service.now}, nil
}

func (service *mcpServiceStub) GetAuthorization(context.Context, mcp.AuthorizationReference) (mcp.AuthorizationAttempt, error) {
	service.authReads.Add(1)
	select {
	case err := <-service.authErrors:
		return mcp.AuthorizationAttempt{}, err
	default:
	}
	finished := service.now.Add(time.Second)
	return mcp.AuthorizationAttempt{
		ID: "auth_1", Server: "docs", Status: mcp.AuthorizationSucceeded,
		CreatedAt: service.now, FinishedAt: &finished,
	}, nil
}

func TestMCPAuthorizationObserverRecoversTransientReadsAndStopsOnAuthoritativeAbsence(t *testing.T) {
	service := newMCPServiceStub()
	service.authErrors = make(chan error, 2)
	service.authErrors <- fmt.Errorf("temporary authorization read failure: %w", agent.ErrDisconnected)
	service.authErrors <- fmt.Errorf("another temporary authorization read failure: %w", agent.ErrDisconnected)
	observer := mcpAuthorizationObserver{
		service: service, pollInterval: time.Nanosecond,
		recovery: retry.Backoff{Base: time.Nanosecond, Maximum: time.Nanosecond},
	}
	initial, err := service.StartAuthorization(t.Context(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	observed, err := observer.observe(t.Context(), initial)
	if err != nil || observed.Status != mcp.AuthorizationSucceeded || service.authReads.Load() != 3 {
		t.Fatalf("observe after transient failures = (%+v, %v), reads %d", observed, err, service.authReads.Load())
	}

	service.authReads.Store(0)
	service.authErrors <- mcp.ErrAuthorizationAttemptNotFound
	if _, err := observer.observe(t.Context(), initial); !errors.Is(err, mcp.ErrAuthorizationAttemptNotFound) {
		t.Fatalf("observe missing attempt = %v, want ErrAuthorizationAttemptNotFound", err)
	}
	if reads := service.authReads.Load(); reads != 1 {
		t.Fatalf("missing attempt reads = %d, want no retry", reads)
	}

	service.authReads.Store(0)
	permanent := errors.New("authorization rejected")
	service.authErrors <- permanent
	if _, err := observer.observe(t.Context(), initial); !errors.Is(err, permanent) {
		t.Fatalf("observe permanent failure = %v, want original error", err)
	}
	if reads := service.authReads.Load(); reads != 1 {
		t.Fatalf("permanent failure reads = %d, want no retry", reads)
	}
}

func TestMCPAuthorizationOutlivesSameSessionProjectionReplacement(t *testing.T) {
	baseService := newMCPServiceStub()
	service := &blockingMCPAuthorizationService{
		mcpServiceStub: baseService,
		started:        make(chan struct{}, 1),
		release:        make(chan struct{}),
		canceled:       make(chan struct{}, 1),
	}
	release := sync.OnceFunc(func() { close(service.release) })
	t.Cleanup(release)

	backend := mock.New()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: backend, SessionID: "ses_demo_1", MCP: service, Changes: source,
	})
	host.Shows(t, "Ask lyra")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/mcp-auth docs")
	host.Press(input.Enter)
	host.Shows(t, "status   pending")
	awaitValue(t, service.started, "MCP authorization observation")

	if _, err := backend.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}
	title := "Authorization refresh installed"
	snapshot, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Title: &title, ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitValue(t, source.applied, "same-session invalidation")
	host.Press(input.Esc)
	host.Shows(t, title)
	select {
	case <-service.canceled:
		t.Fatal("same-session projection replacement canceled the application-owned MCP authorization")
	default:
	}

	release()
	host.Shows(t, "MCP authorization succeeded · docs")
	if service.authReads.Load() == 0 {
		t.Fatal("MCP authorization did not resume after the session projection replacement")
	}
	stop()
}

func TestMCPReadersFormsAndLifecycleCommands(t *testing.T) {
	service := newMCPServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), MCP: service})
	host.Shows(t, "Ask lyra")
	host.Type("/mcp")
	host.Press(input.Enter)
	host.Shows(t, "docs · connected · 1 tools")
	host.Shows(t, "Bearer ****")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/mcp-tools docs")
	host.Press(input.Enter)
	host.Shows(t, "docs/search")
	host.Shows(t, "Input schema")
	host.Shows(t, "object")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")

	host.Type("/mcp-create")
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server")
	host.Type("private-docs")
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 2/3")
	host.Shows(t, "HTTP URL")
	host.Type("https://private.example/tools")
	host.Press(input.Tab)
	host.Press(input.Down)
	host.Press(input.Tab)
	secret := "MCP_SECRET_42"
	host.Type(secret)
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("MCP form did not survive a minimal viewport")
	}
	host.Shows(t, "Create MCP server")
	if strings.Contains(host.Frames(), secret) {
		t.Fatal("MCP authorization secret appeared in terminal frames")
	}
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 3/3")
	host.Shows(t, "Disabled tools")
	host.Press(input.Enter)
	host.Shows(t, "private-docs · disconnected")
	created := <-service.created
	if created.Connection.Authorization == nil || created.Connection.Authorization.Value != secret {
		t.Fatalf("created MCP candidate = %+v", created)
	}
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")

	host.Type("/mcp-edit docs")
	host.Press(input.Enter)
	host.Shows(t, "Configure MCP server · docs")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "Configure MCP server · docs · 2/2")
	host.Press(input.Enter)
	host.Shows(t, "docs · disabled")
	updated := <-service.updated
	if updated.Enabled == nil || *updated.Enabled {
		t.Fatalf("MCP update = %+v", updated)
	}
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/mcp-reconnect docs")
	host.Press(input.Enter)
	host.Shows(t, "requesting MCP reconnect docs accepted")
	if reconnected := <-service.reconnected; reconnected != "docs" {
		t.Fatalf("reconnected = %q", reconnected)
	}

	host.Type("/mcp-auth docs")
	host.Press(input.Enter)
	host.Shows(t, "complete the sign-in in your browser")
	host.Shows(t, "status   succeeded")
	if service.authReads.Load() == 0 {
		t.Fatal("MCP authorization was not polled")
	}
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/mcp-delete docs")
	host.Press(input.Enter)
	host.Shows(t, "Delete MCP server")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "deleting MCP server docs accepted")
	if deleted := <-service.deleted; deleted != "docs" {
		t.Fatalf("deleted = %q", deleted)
	}
	stop()
}

func TestMCPProbeValidatesAnUnpersistedCandidateAcrossResize(t *testing.T) {
	service := newMCPServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), MCP: service})
	host.Shows(t, "Ask lyra")
	host.Type("/mcp-probe")
	host.Press(input.Enter)
	host.Shows(t, "Test MCP candidate · 1/3")
	host.Type("probe-docs")
	host.Press(input.Enter)
	host.Shows(t, "Test MCP candidate · 2/3")
	host.Type("https://probe.example/tools")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("MCP probe form did not survive a minimal viewport")
	}
	host.Shows(t, "Test MCP candidate · 2/3")
	host.Press(input.Enter)
	host.Shows(t, "Test MCP candidate · 3/3")
	host.Press(input.Enter)
	host.Shows(t, "MCP candidate is reachable · probe-docs")
	probed := awaitValue(t, service.probed, "MCP candidate probe")
	if probed.Name != "probe-docs" || probed.Connection.Transport != mcp.StreamableHTTP ||
		probed.Connection.URL != "https://probe.example/tools" {
		t.Fatalf("probed MCP candidate = %+v", probed)
	}
	servers, err := service.Servers(t.Context())
	if err != nil || len(servers) != 1 || servers[0].Name != "docs" {
		t.Fatalf("MCP probe persisted candidate: servers=(%+v, %v)", servers, err)
	}
	stop()
}

func TestMCPStdioWizardKeepsEveryFieldVisibleAndSecretsMasked(t *testing.T) {
	service := newMCPServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), MCP: service})
	host.Shows(t, "Ask lyra")
	host.Type("/mcp-create")
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 1/3")
	host.Type("local-tools")
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Down)
	showsPlain(t, host, "● stdio process")
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 2/3")
	host.Shows(t, "stdio command")
	host.Type("local-mcp")
	host.Press(input.Tab)
	host.Type(`["--stdio"]`)
	host.Press(input.Tab)
	host.Press(input.Down)
	host.Press(input.Tab)
	secret := `{"TOKEN":"MCP_STDIO_SECRET"}`
	host.Type(secret)
	host.Press(input.Tab)
	host.Type("/tmp")
	if !host.Resize(44, 18) || !host.Resize(96, 28) {
		t.Fatal("stdio MCP step did not survive a narrow viewport round trip")
	}
	if strings.Contains(host.Frames(), "MCP_STDIO_SECRET") {
		t.Fatal("stdio environment secret appeared in terminal frames")
	}
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 3/3")
	host.Press(input.Tab)
	host.Type("read, read")
	host.Press(input.Enter)
	host.Shows(t, `tool "read" is duplicated`)
	select {
	case candidate := <-service.created:
		t.Fatalf("invalid MCP policy reached the service: %+v", candidate)
	default:
	}
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("read")
	host.Press(input.Enter)
	host.Shows(t, "local-tools · disconnected")
	created := <-service.created
	if created.Connection.Transport != mcp.Stdio || created.Connection.Command != "local-mcp" ||
		!slices.Equal(created.Connection.Args, []string{"--stdio"}) || created.Connection.Directory != "/tmp" ||
		created.Connection.Environment == nil || created.Connection.Environment.Value["TOKEN"] != "MCP_STDIO_SECRET" ||
		!slices.Equal(created.DisabledTools, []string{"read"}) {
		t.Fatalf("created stdio MCP candidate = %+v", created)
	}

	host.Press(input.Esc)
	stop()
}

func TestMCPChangedRefetchesTheOpenServerReader(t *testing.T) {
	service := newMCPServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.MCPChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), MCP: service, Changes: source})
	host.Shows(t, "Ask lyra")
	subscription := awaitValue(t, source.subscription, "MCP invalidation subscription")
	if len(subscription.Topics) != 1 || subscription.Topics[0] != changefeed.MCPChanged {
		t.Fatalf("MCP subscription = %+v", subscription)
	}
	host.Type("/mcp")
	host.Press(input.Enter)
	host.Shows(t, "Documentation")
	service.mu.Lock()
	service.servers[0].Description = "Updated documentation server"
	service.mu.Unlock()
	source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.MCPChanged), Sequence: 1, ServerIDs: []string{"docs"}}
	awaitSignal(t, source.applied, "mcp.changed delivery")
	host.Shows(t, "Updated documentation server")
	stop()
}
