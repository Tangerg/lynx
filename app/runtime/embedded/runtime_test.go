package embedded

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestResolveConfigUsesExplicitStableDefaults(t *testing.T) {
	data := t.TempDir()
	home := t.TempDir()
	resolved, err := resolveConfig(Config{DataDirectory: data, UserHomePath: home})
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if resolved.DefaultWorkspacePath != home {
		t.Fatalf("default workspace = %q, want user home %q", resolved.DefaultWorkspacePath, home)
	}
	if len(resolved.ConfigDirectories) != 1 || resolved.ConfigDirectories[0] != data {
		t.Fatalf("config directories = %v, want [%s]", resolved.ConfigDirectories, data)
	}

	for _, test := range []struct {
		name   string
		config Config
	}{
		{name: "missing data directory", config: Config{UserHomePath: home}},
		{name: "relative data directory", config: Config{DataDirectory: "data", UserHomePath: home}},
		{name: "relative home", config: Config{DataDirectory: data, UserHomePath: "home"}},
		{name: "relative workspace", config: Config{DataDirectory: data, UserHomePath: home, DefaultWorkspacePath: "workspace"}},
		{name: "relative config directory", config: Config{DataDirectory: data, UserHomePath: home, ConfigDirectories: []string{"config"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := resolveConfig(test.config); err == nil {
				t.Fatal("resolveConfig accepted an ambiguous host path")
			}
		})
	}
}

func TestRuntimeOpenCallIdempotencyStreamAndClose(t *testing.T) {
	t.Setenv("LYRA_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("LYRA_MCP_SERVERS", "")
	t.Setenv("LYRA_A2A_AGENTS", "")
	t.Setenv("LYRA_A2A_RPC_ORIGINS", "")

	config := Config{
		DataDirectory:        t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		UserHomePath:         t.TempDir(),
		ConfigDirectories:    []string{t.TempDir()},
	}
	runtime, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	discovery, err := runtime.Discover(t.Context(), CallOptions{})
	if err != nil || discovery.ProtocolVersion != protocol.ProtocolVersion {
		t.Fatalf("Discover = (%+v, %v)", discovery, err)
	}
	if _, err := runtime.Discover(t.Context(), CallOptions{RequestMeta: protocol.RequestMeta{
		ProtocolVersion: "1900-01-01",
	}}); !errors.Is(err, protocol.ErrInvalidProtocolVersion) {
		t.Fatalf("unsupported protocol error = %v", err)
	} else {
		var problem protocol.ProblemError
		if !errors.As(err, &problem) || problem.Problem().Type != protocol.ErrInvalidProtocolVersion.Error() {
			t.Fatalf("structured unsupported protocol error = %T %v", err, err)
		}
	}

	create := protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: config.DefaultWorkspacePath},
		Title:     "embedded",
	}
	first, err := runtime.CreateSession(t.Context(), create, CommandOptions{IdempotencyKey: "create-once"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	replayed, err := runtime.CreateSession(t.Context(), create, CommandOptions{IdempotencyKey: "create-once"})
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("replayed CreateSession = (%+v, %v), want %s", replayed, err, first.ID)
	}
	create.Title = "different"
	if _, err := runtime.CreateSession(t.Context(), create, CommandOptions{IdempotencyKey: "create-once"}); !errors.Is(err, protocol.ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict error = %v", err)
	}

	second, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("second Open sharing data directory: %v", err)
	}

	_, events, err := second.SubscribeRuntime(t.Context(), protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged},
	}, SubscriptionOptions{})
	if err != nil {
		t.Fatalf("SubscribeRuntime: %v", err)
	}
	streamDone := make(chan struct{})
	eventReceived := make(chan protocol.RuntimeEvent, 1)
	go func() {
		defer close(streamDone)
		for event, err := range events {
			if err != nil {
				return
			}
			eventReceived <- event
			return
		}
	}()
	if _, err := runtime.CreateSession(t.Context(), protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: config.DefaultWorkspacePath},
		Title:     "notifies",
	}, CommandOptions{IdempotencyKey: "create-notification"}); err != nil {
		t.Fatalf("CreateSession for notification: %v", err)
	}
	select {
	case event := <-eventReceived:
		if event.Type != protocol.RuntimeResync ||
			!slices.Equal(event.Topics, []protocol.RuntimeTopic{protocol.TopicSessionsChanged}) {
			t.Fatalf("cross-Runtime event = %+v, want scoped resync", event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("peer Runtime commit produced no scoped resync")
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second Runtime: %v", err)
	}
	<-streamDone

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := runtime.Discover(t.Context(), CallOptions{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Discover after Close error = %v, want ErrClosed", err)
	}

	reopened, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened Runtime: %v", err)
	}
}

func TestResolveConfigCleansPaths(t *testing.T) {
	root := t.TempDir()
	resolved, err := resolveConfig(Config{
		DataDirectory:        filepath.Join(root, "data", "..", "data"),
		DefaultWorkspacePath: filepath.Join(root, "workspace", "."),
		UserHomePath:         filepath.Join(root, "home", "."),
	})
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if resolved.DataDirectory != filepath.Join(root, "data") ||
		resolved.DefaultWorkspacePath != filepath.Join(root, "workspace") ||
		resolved.UserHomePath != filepath.Join(root, "home") {
		t.Fatalf("resolved paths = %+v", resolved)
	}
}
