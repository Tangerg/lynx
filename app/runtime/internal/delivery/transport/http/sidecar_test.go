package http_test

import (
	"context"
	"encoding/json"
	netHTTP "net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	lyrahttp "github.com/Tangerg/lynx/app/runtime/internal/delivery/transport/http"
)

func newProbeServer(t *testing.T, probes ...lyrahttp.HealthProbe) *httptest.Server {
	t.Helper()
	srv, err := lyrahttp.NewServer(lyrahttp.Config{
		Runtime:         &fakeRuntime{},
		Addr:            ":0",
		ServerInfo:      protocol.ServerInfo{Name: "lyra-test", Version: "0.0.0", Cwd: "/secret/project", Home: "/secret/home"},
		ProtocolVersion: testProtocolVersion,
		HealthProbes:    probes,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

func TestInfoIsMinimalAndTyped(t *testing.T) {
	ts := newProbeServer(t)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/info")
	if err != nil {
		t.Fatalf("get info: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Protocol struct {
			Current      string `json:"current"`
			MinSupported string `json:"minSupported"`
		} `json:"protocol"`
		Server struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"server"`
		Transport string `json:"transport"`
		Endpoints struct {
			RPC       string `json:"rpc"`
			Info      string `json:"info"`
			Liveness  string `json:"liveness"`
			Readiness string `json:"readiness"`
		} `json:"endpoints"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Protocol.Current != testProtocolVersion || body.Protocol.MinSupported != testProtocolVersion {
		t.Fatalf("protocol = %+v", body.Protocol)
	}
	if body.Server.Name != "lyra-test" || body.Server.Version != "0.0.0" {
		t.Fatalf("server = %+v", body.Server)
	}
	if body.Transport != "http" {
		t.Fatalf("transport = %q", body.Transport)
	}
	if body.Endpoints.RPC != "/v2/rpc" || body.Endpoints.Liveness != "/v2/health/live" || body.Endpoints.Readiness != "/v2/health/ready" {
		t.Fatalf("endpoints = %+v", body.Endpoints)
	}
}

func TestInfoDoesNotExposeRuntimePathsOrCapabilities(t *testing.T) {
	ts := newProbeServer(t)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/info")
	if err != nil {
		t.Fatalf("get info: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, field := range []string{"serverInfo", "capabilities", "agentDocs", "cwd", "home"} {
		if _, ok := body[field]; ok {
			t.Fatalf("sensitive field %q present in info: %+v", field, body)
		}
	}
}

func TestLivenessDoesNotCallReadinessProbes(t *testing.T) {
	var calls atomic.Int32
	ts := newProbeServer(t, lyrahttp.HealthProbe{
		Name: "storage",
		Probe: func(context.Context) lyrahttp.HealthCheck {
			calls.Add(1)
			return lyrahttp.HealthCheck{Status: lyrahttp.HealthUnhealthy}
		},
	})
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/health/live")
	if err != nil {
		t.Fatalf("get liveness: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != netHTTP.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if calls.Load() != 0 {
		t.Fatalf("readiness probe calls = %d", calls.Load())
	}
}

func TestReadinessReportsWorstProbe(t *testing.T) {
	ts := newProbeServer(t,
		lyrahttp.HealthProbe{Name: "runtime", Probe: func(context.Context) lyrahttp.HealthCheck {
			return lyrahttp.HealthCheck{Status: lyrahttp.HealthOK}
		}},
		lyrahttp.HealthProbe{Name: "storage", Probe: func(context.Context) lyrahttp.HealthCheck {
			return lyrahttp.HealthCheck{Status: lyrahttp.HealthUnhealthy}
		}},
	)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/health/ready")
	if err != nil {
		t.Fatalf("get readiness: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != netHTTP.StatusServiceUnavailable {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "unhealthy" || body.Checks["runtime"] != "ok" || body.Checks["storage"] != "unhealthy" {
		t.Fatalf("readiness = %+v", body)
	}
}

func TestReadinessContainsProbePanic(t *testing.T) {
	ts := newProbeServer(t, lyrahttp.HealthProbe{
		Name: "panic",
		Probe: func(context.Context) lyrahttp.HealthCheck {
			panic("boom")
		},
	})
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/health/ready")
	if err != nil {
		t.Fatalf("get readiness: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != netHTTP.StatusServiceUnavailable {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestNewServerRejectsAmbiguousHealthProbes(t *testing.T) {
	base := lyrahttp.Config{
		Runtime:         &fakeRuntime{},
		Addr:            ":0",
		ServerInfo:      protocol.ServerInfo{Name: "lyra-test", Version: "0.0.0"},
		ProtocolVersion: testProtocolVersion,
	}
	tests := []struct {
		name   string
		probes []lyrahttp.HealthProbe
	}{
		{name: "missing function", probes: []lyrahttp.HealthProbe{{Name: "storage"}}},
		{name: "duplicate name", probes: []lyrahttp.HealthProbe{
			{Name: "storage", Probe: func(context.Context) lyrahttp.HealthCheck { return lyrahttp.HealthCheck{} }},
			{Name: "storage", Probe: func(context.Context) lyrahttp.HealthCheck { return lyrahttp.HealthCheck{} }},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			cfg.HealthProbes = tt.probes
			if _, err := lyrahttp.NewServer(cfg); err == nil {
				t.Fatal("NewServer accepted ambiguous health probes")
			}
		})
	}
}
