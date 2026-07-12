package integrations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func TestMCPLiveStatusAndToolsUsePorts(t *testing.T) {
	live := &fakeMCPLive{
		statuses: []mcpserver.ConnectionStatus{{Name: "fs", Status: "connected"}},
		tools:    []mcpserver.ToolInfo{{Server: "fs", Name: "read"}},
	}
	c := New(Config{MCPLive: live})

	if got := c.MCPServerStatuses(); len(got) != 1 || got[0].Name != "fs" {
		t.Fatalf("MCPServerStatuses = %+v", got)
	}
	tools, err := c.MCPTools(context.Background(), "fs")
	if err != nil {
		t.Fatalf("MCPTools err = %v", err)
	}
	if live.toolsServer != "fs" || len(tools) != 1 || tools[0].Name != "read" {
		t.Fatalf("tools server=%q tools=%+v", live.toolsServer, tools)
	}
}

// TestMCPLiveConnectionCommandsUsePort: reconnect/authorize are fire-and-forget —
// they validate the name synchronously, then dial on the component task group and
// publish the settled frame. The test waits on the settled notification (which
// runs after the dial) before asserting the live port was driven with the name.
func TestMCPLiveConnectionCommandsUsePort(t *testing.T) {
	live := &fakeMCPLive{statuses: []mcpserver.ConnectionStatus{{Name: "fs"}, {Name: "github"}}}
	settled := make(chan string, 2)
	c := New(Config{MCPLive: live, MCPStatus: func(_ context.Context, server string, connecting bool) {
		if !connecting {
			settled <- server
		}
	}})
	defer c.Close()

	if err := c.ReconnectMCPServer(context.Background(), "fs"); err != nil {
		t.Fatalf("ReconnectMCPServer err = %v", err)
	}
	if got := <-settled; got != "fs" {
		t.Fatalf("settled server = %q, want fs", got)
	}
	if err := c.AuthorizeMCPServer(context.Background(), "github"); err != nil {
		t.Fatalf("AuthorizeMCPServer err = %v", err)
	}
	if got := <-settled; got != "github" {
		t.Fatalf("settled server = %q, want github", got)
	}

	if live.reconnectName != "fs" || live.authorizeName != "github" {
		t.Fatalf("reconnect=%q authorize=%q", live.reconnectName, live.authorizeName)
	}

	if err := c.ReconnectMCPServer(context.Background(), "ghost"); !errors.Is(err, mcpserver.ErrUnknownServer) {
		t.Fatalf("reconnect unknown = %v, want ErrUnknownServer", err)
	}
}

// TestReconnectMCPServerDetachedButComponentOwned: a dial detaches the caller's
// cancellation (a returning RPC must not abort it) while preserving its trace
// values, and is canceled + joined by Coordinator.Close; a reconnect requested
// after Close reports errClosed. This is the component-owned lifecycle §10.2/§10.3
// the delivery layer used to hold on its own task group.
func TestReconnectMCPServerDetachedButComponentOwned(t *testing.T) {
	type ctxKey struct{}
	live := &blockingMCPLive{
		fakeMCPLive: fakeMCPLive{statuses: []mcpserver.ConnectionStatus{{Name: "fs"}}},
		started:     make(chan bool, 1),
		stopped:     make(chan struct{}),
		wantValue:   func(ctx context.Context) bool { return ctx.Value(ctxKey{}) == "trace" },
	}
	c := New(Config{MCPLive: live})

	reqCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "trace"))
	cancelRequest() // the request is done — the dial must keep running

	if err := c.ReconnectMCPServer(reqCtx, "fs"); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if detached := <-live.started; !detached {
		t.Fatal("dial context did not detach request cancellation or preserve values")
	}

	c.Close()
	select {
	case <-live.stopped:
	case <-time.After(time.Second):
		t.Fatal("Coordinator.Close did not cancel and join the dial")
	}
	if err := c.ReconnectMCPServer(context.Background(), "fs"); !errors.Is(err, errClosed) {
		t.Fatalf("reconnect after Close = %v, want errClosed", err)
	}
}

func TestTestMCPServerUsesLiveRegistryPort(t *testing.T) {
	live := &fakeMCPLive{}
	c := New(Config{MCPLive: live})

	err := c.TestMCPServer(context.Background(), mcpserver.Server{
		Name:      "fs",
		Transport: mcpserver.TransportStdio,
		Command:   "mcp-fs",
		Args:      []string{"--root", "/repo"},
		Env:       map[string]string{"A": "1"},
	})
	if err != nil {
		t.Fatalf("TestMCPServer err = %v", err)
	}
	if live.probe.Name != "fs" || live.probe.Command != "mcp-fs" || len(live.probe.Env) != 1 || live.probe.Env[0] != "A=1" {
		t.Fatalf("probe config = %+v", live.probe)
	}
}

type fakeMCPLive struct {
	statuses []mcpserver.ConnectionStatus
	tools    []mcpserver.ToolInfo

	toolsServer string

	reconnectName string
	authorizeName string

	probe      mcpserver.LiveConfig
	configure  mcpserver.LiveConfig
	removeName string
}

func (f *fakeMCPLive) MCPServerStatuses() []mcpserver.ConnectionStatus { return f.statuses }

func (f *fakeMCPLive) MCPTools(_ context.Context, server string) ([]mcpserver.ToolInfo, error) {
	f.toolsServer = server
	return f.tools, nil
}

func (f *fakeMCPLive) ReconnectMCPServer(_ context.Context, name string) error {
	f.reconnectName = name
	return nil
}

func (f *fakeMCPLive) AuthorizeMCPServer(_ context.Context, name string) error {
	f.authorizeName = name
	return nil
}

func (f *fakeMCPLive) ProbeMCPServer(_ context.Context, cfg mcpserver.LiveConfig) error {
	f.probe = cfg
	return nil
}

func (f *fakeMCPLive) ConfigureMCPServer(_ context.Context, cfg mcpserver.LiveConfig) error {
	f.configure = cfg
	return nil
}

func (f *fakeMCPLive) RemoveMCPServer(_ context.Context, name string) {
	f.removeName = name
}

// blockingMCPLive is a fakeMCPLive whose dial blocks on its context until Close,
// so a test can observe the detach + component-owned-cancellation contract.
type blockingMCPLive struct {
	fakeMCPLive
	started   chan bool
	stopped   chan struct{}
	wantValue func(context.Context) bool
}

func (f *blockingMCPLive) ReconnectMCPServer(ctx context.Context, _ string) error {
	f.started <- ctx.Err() == nil && f.wantValue(ctx)
	<-ctx.Done()
	close(f.stopped)
	return ctx.Err()
}
