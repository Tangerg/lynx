package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

type lifecycleRuntime struct{ protocol.Runtime }

func newLifecycleServer(t *testing.T, configure func(*Config)) *Server {
	t.Helper()
	cfg := Config{
		Runtime:         lifecycleRuntime{},
		Addr:            "127.0.0.1:0",
		ServerInfo:      protocol.ServerInfo{Name: "test", Version: "1"},
		ProtocolVersion: "test",
	}
	if configure != nil {
		configure(&cfg)
	}
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func TestShutdownBeforeStartPreventsListen(t *testing.T) {
	srv := newLifecycleServer(t, nil)
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := srv.Start(); !errors.Is(err, stdhttp.ErrServerClosed) {
		t.Fatalf("Start after Shutdown = %v, want http.ErrServerClosed", err)
	}
	if err := srv.Start(); err == nil {
		t.Fatal("second Start unexpectedly succeeded")
	}
}

func TestNewServerSnapshotsConfig(t *testing.T) {
	origins := []string{"https://before.example"}
	probes := []HealthProbe{{Name: "before"}}
	capabilities := protocol.ServerCapabilities{
		Events:           []protocol.StreamEventType{"before"},
		StreamingMethods: []string{"before"},
		Features:         map[string]protocol.FeatureFlag{"before": true},
		Providers:        []string{"before"},
	}
	srv := newLifecycleServer(t, func(cfg *Config) {
		cfg.CORSOrigins = origins
		cfg.HealthProbes = probes
		cfg.Capabilities = capabilities
	})

	origins[0] = "https://after.example"
	probes[0].Name = "after"
	capabilities.Events[0] = "after"
	capabilities.StreamingMethods[0] = "after"
	capabilities.Features["before"] = false
	capabilities.Providers[0] = "after"

	if srv.corsOrigins[0] != "https://before.example" || srv.healthProbes[0].Name != "before" {
		t.Fatal("server retained caller-owned transport configuration")
	}
	got := srv.info.Capabilities
	if got.Events[0] != "before" || got.StreamingMethods[0] != "before" || got.Features["before"] != true || got.Providers[0] != "before" {
		t.Fatalf("server retained caller-owned capabilities: %+v", got)
	}
}

func TestHealthBudgetDoesNotWaitForUncooperativeProbe(t *testing.T) {
	blocked := make(chan struct{})
	probes := []HealthProbe{
		{Name: "blocked", Probe: func(context.Context) HealthCheck {
			<-blocked
			return HealthCheck{Status: HealthOK}
		}},
		{Name: "fast", Probe: func(context.Context) HealthCheck {
			return HealthCheck{Status: HealthOK}
		}},
	}

	started := time.Now()
	overall, checks := runHealthProbesWithBudget(context.Background(), probes, 20*time.Millisecond)
	elapsed := time.Since(started)
	close(blocked)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("health probes exceeded hard budget: %v", elapsed)
	}
	if overall != HealthUnhealthy || checks["blocked"] != HealthUnhealthy || checks["fast"] != HealthOK {
		t.Fatalf("overall/checks = %q/%v", overall, checks)
	}
}
