package terminal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/mcp"
)

type mcpServiceStub struct {
	mu          sync.Mutex
	servers     []mcp.Server
	created     chan mcp.Candidate
	updated     chan mcp.ServerUpdate
	deleted     chan string
	reconnected chan string
	authReads   atomic.Int32
	now         time.Time
}

func newMCPServiceStub() *mcpServiceStub {
	count := 1
	return &mcpServiceStub{
		servers: []mcp.Server{{
			Name: "docs", Description: "Documentation", TimeoutSeconds: 15,
			Connection: mcp.Connection{Transport: mcp.StreamableHTTP, URL: "https://mcp.example/tools", AuthorizationMasked: "Bearer ****"},
			State:      mcp.State{Type: mcp.Connected, ToolCount: &count},
		}},
		created: make(chan mcp.Candidate, 1), updated: make(chan mcp.ServerUpdate, 1),
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

func (*mcpServiceStub) TestServer(_ context.Context, candidate mcp.Candidate) (mcp.TestResult, error) {
	if err := candidate.Validate(); err != nil {
		return mcp.TestResult{}, err
	}
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

func (service *mcpServiceStub) GetAuthorization(context.Context, string) (mcp.AuthorizationAttempt, error) {
	service.authReads.Add(1)
	finished := service.now.Add(time.Second)
	return mcp.AuthorizationAttempt{
		ID: "auth_1", Server: "docs", Status: mcp.AuthorizationSucceeded,
		CreatedAt: service.now, FinishedAt: &finished,
	}, nil
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
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Tab)
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
