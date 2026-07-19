package http

import (
	"bytes"
	"context"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

type lifecycleRuntime struct{ protocol.Runtime }

type streamingLifecycleRuntime struct {
	protocol.Runtime
	subscribed chan struct{}
}

func (r *streamingLifecycleRuntime) WorkspaceSubscribe(
	context.Context,
	protocol.WorkspaceSubscribeRequest,
) (*protocol.WorkspaceSubscribeResponse, <-chan protocol.WorkspaceEvent, error) {
	close(r.subscribed)
	return &protocol.WorkspaceSubscribeResponse{}, make(chan protocol.WorkspaceEvent), nil
}

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
	if err := srv.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := srv.Start(); !errors.Is(err, stdhttp.ErrServerClosed) {
		t.Fatalf("Start after Shutdown = %v, want http.ErrServerClosed", err)
	}
	if err := srv.Start(); err == nil {
		t.Fatal("second Start unexpectedly succeeded")
	}
}

func TestShutdownCancelsLongLivedTransportHandler(t *testing.T) {
	waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Second)
	defer cancelWait()
	runtime := &streamingLifecycleRuntime{subscribed: make(chan struct{})}
	srv := newLifecycleServer(t, func(cfg *Config) {
		cfg.Runtime = runtime
	})
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":"1","method":"workspace.subscribe","params":{}}`)
	req := httptest.NewRequest(stdhttp.MethodPost, "/v2/rpc", body)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")

	handled := make(chan struct{})
	go func() {
		defer close(handled)
		srv.Handler().ServeHTTP(httptest.NewRecorder(), req)
	}()

	select {
	case <-runtime.subscribed:
	case <-waitCtx.Done():
		t.Fatal("workspace subscription did not start")
	}
	if err := srv.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case <-handled:
	case <-waitCtx.Done():
		t.Fatal("streaming handler survived server shutdown")
	}
}

func TestNewServerSnapshotsConfig(t *testing.T) {
	origins := []string{"https://before.example"}
	probes := []HealthProbe{{Name: "before", Probe: func(context.Context) HealthCheck {
		return HealthCheck{Status: HealthOK}
	}}}
	srv := newLifecycleServer(t, func(cfg *Config) {
		cfg.CORSOrigins = origins
		cfg.HealthProbes = probes
	})

	origins[0] = "https://after.example"
	probes[0].Name = "after"

	if srv.corsOrigins[0] != "https://before.example" || srv.healthProbes[0].name != "before" {
		t.Fatal("server retained caller-owned transport configuration")
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
	overall, checks := runHealthProbesWithBudget(t.Context(), newHealthProbeRunners(probes), 20*time.Millisecond)
	elapsed := time.Since(started)
	close(blocked)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("health probes exceeded hard budget: %v", elapsed)
	}
	if overall != HealthUnhealthy || checks["blocked"] != HealthUnhealthy || checks["fast"] != HealthOK {
		t.Fatalf("overall/checks = %q/%v", overall, checks)
	}
}

func TestHealthProbeRunnerLimitsUncooperativeProbeToOneInvocation(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	runners := newHealthProbeRunners([]HealthProbe{{
		Name: "blocked",
		Probe: func(context.Context) HealthCheck {
			started <- struct{}{}
			<-release
			return HealthCheck{Status: HealthOK}
		},
	}})

	for range 2 {
		overall, checks := runHealthProbesWithBudget(t.Context(), runners, 20*time.Millisecond)
		if overall != HealthUnhealthy || checks["blocked"] != HealthUnhealthy {
			t.Fatalf("overall/checks = %q/%v", overall, checks)
		}
	}
	select {
	case <-started:
	default:
		t.Fatal("probe was never invoked")
	}
	select {
	case <-started:
		t.Fatal("uncooperative probe was invoked more than once while still running")
	default:
	}

	close(release)
	deadline := time.Now().Add(time.Second)
	for {
		runners[0].mu.Lock()
		running := runners[0].current != nil
		runners[0].mu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("released probe did not finish")
		}
		runtime.Gosched()
	}

	overall, checks := runHealthProbesWithBudget(t.Context(), runners, time.Second)
	if overall != HealthOK || checks["blocked"] != HealthOK {
		t.Fatalf("restarted overall/checks = %q/%v", overall, checks)
	}
	select {
	case <-started:
	default:
		t.Fatal("probe did not restart after its prior invocation finished")
	}
}

type healthContextKey struct{}

func TestHealthProbeInvocationOutlivesStartingRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runners := newHealthProbeRunners([]HealthProbe{{
		Name: "detached",
		Probe: func(ctx context.Context) HealthCheck {
			close(started)
			<-release
			if ctx.Err() != nil || ctx.Value(healthContextKey{}) != "preserved" {
				return HealthCheck{Status: HealthUnhealthy}
			}
			return HealthCheck{Status: HealthOK}
		},
	}})
	requestCtx, cancelRequest := context.WithCancel(context.WithValue(t.Context(), healthContextKey{}, "preserved"))
	invocation := runners[0].start(requestCtx, time.Second)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("health probe did not start")
	}
	cancelRequest()
	close(release)
	select {
	case <-invocation.done:
	case <-time.After(time.Second):
		t.Fatal("health probe did not finish")
	}

	if invocation.check.Status != HealthOK {
		t.Fatalf("probe status = %q, want ok after starting request was canceled", invocation.check.Status)
	}
}

func TestHealthProbeRejectsUnknownStatus(t *testing.T) {
	runners := newHealthProbeRunners([]HealthProbe{{
		Name: "invalid",
		Probe: func(context.Context) HealthCheck {
			return HealthCheck{Status: "unexpected"}
		},
	}})

	overall, checks := runHealthProbesWithBudget(t.Context(), runners, time.Second)
	if overall != HealthUnhealthy || checks["invalid"] != HealthUnhealthy {
		t.Fatalf("overall/checks = %q/%v, want unhealthy", overall, checks)
	}
}
