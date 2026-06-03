package http_test

import (
	"context"
	"encoding/json"
	netHTTP "net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	lyrahttp "github.com/Tangerg/lynx/lyra/rpc/transport/http"
)

// newProbeServer builds a test server with the supplied probes.
// LocalToken / CORS deliberately left empty — sidecar tests aren't
// concerned with those layers.
func newProbeServer(t *testing.T, probes ...lyrahttp.HealthProbe) *httptest.Server {
	t.Helper()
	srv, err := lyrahttp.NewServer(lyrahttp.Config{
		Runtime:         &fakeRuntime{},
		Addr:            ":0",
		ServerInfo:      protocol.ServerInfo{Name: "lyra-test", Version: "0.0.0"},
		ProtocolVersion: testProtocolVersion,
		Capabilities:    protocol.ServerCapabilities{},
		HealthProbes:    probes,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

// TestInfoExposesOpsMetadata confirms /v2/info includes serverID,
// transport, and the endpoint route table — the bits oncall reaches
// for first when wiring up a new client.
func TestInfoExposesOpsMetadata(t *testing.T) {
	ts := newProbeServer(t)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/info")
	if err != nil {
		t.Fatalf("get info: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		ServerID  string            `json:"serverID"`
		Transport string            `json:"transport"`
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ServerID != "lyra-test/0.0.0" {
		t.Fatalf("serverID = %q", body.ServerID)
	}
	if body.Transport != "http" {
		t.Fatalf("transport = %q, want http", body.Transport)
	}
	for _, key := range []string{"rpc", "stream", "info", "health"} {
		if body.Endpoints[key] == "" {
			t.Fatalf("endpoints[%q] missing; got %+v", key, body.Endpoints)
		}
	}
}

// TestHealthNoProbes confirms the default-empty-probes case: 200 +
// status:"ok", checks field omitted.
func TestHealthNoProbes(t *testing.T) {
	ts := newProbeServer(t)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		OK     bool           `json:"ok"`
		Status string         `json:"status"`
		Checks map[string]any `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.OK {
		t.Fatalf("ok = false, want true")
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
	if body.Checks != nil {
		t.Fatalf("checks should be omitted when no probes; got %+v", body.Checks)
	}
}

// TestHealthAllOK confirms all-ok probes → 200 + status:"ok" + checks
// map populated with each probe's name and "ok" status.
func TestHealthAllOK(t *testing.T) {
	ts := newProbeServer(t,
		lyrahttp.HealthProbe{
			Name: "runtime",
			Probe: func(_ context.Context) lyrahttp.HealthCheck {
				return lyrahttp.HealthCheck{Status: lyrahttp.HealthOK}
			},
		},
		lyrahttp.HealthProbe{
			Name: "storage",
			Probe: func(_ context.Context) lyrahttp.HealthCheck {
				return lyrahttp.HealthCheck{Status: lyrahttp.HealthOK}
			},
		},
	)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q", body.Status)
	}
	if body.Checks["runtime"] != "ok" || body.Checks["storage"] != "ok" {
		t.Fatalf("checks = %+v", body.Checks)
	}
}

// TestHealthDegraded confirms: one degraded probe ⇒ 503 +
// status:"degraded" + that probe's check labeled degraded.
func TestHealthDegraded(t *testing.T) {
	ts := newProbeServer(t,
		lyrahttp.HealthProbe{
			Name: "runtime",
			Probe: func(_ context.Context) lyrahttp.HealthCheck {
				return lyrahttp.HealthCheck{Status: lyrahttp.HealthOK}
			},
		},
		lyrahttp.HealthProbe{
			Name: "providers",
			Probe: func(_ context.Context) lyrahttp.HealthCheck {
				return lyrahttp.HealthCheck{Status: lyrahttp.HealthDegraded}
			},
		},
	)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	var body struct {
		OK     bool              `json:"ok"`
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.OK {
		t.Fatalf("ok = true, want false for degraded")
	}
	if body.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", body.Status)
	}
	if body.Checks["providers"] != "degraded" {
		t.Fatalf("providers check = %q", body.Checks["providers"])
	}
	if body.Checks["runtime"] != "ok" {
		t.Fatalf("runtime check = %q", body.Checks["runtime"])
	}
}

// TestHealthWorstWins confirms unhealthy > degraded — both present
// surfaces as unhealthy.
func TestHealthWorstWins(t *testing.T) {
	ts := newProbeServer(t,
		lyrahttp.HealthProbe{
			Name: "providers",
			Probe: func(_ context.Context) lyrahttp.HealthCheck {
				return lyrahttp.HealthCheck{Status: lyrahttp.HealthDegraded}
			},
		},
		lyrahttp.HealthProbe{
			Name: "storage",
			Probe: func(_ context.Context) lyrahttp.HealthCheck {
				return lyrahttp.HealthCheck{Status: lyrahttp.HealthUnhealthy}
			},
		},
	)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "unhealthy" {
		t.Fatalf("status = %q, want unhealthy", body.Status)
	}
}

// TestInfoExposesAgentDocs confirms /v2/info surfaces the
// AgentDocsLister's output under the `agentDocs` field.
func TestInfoExposesAgentDocs(t *testing.T) {
	srv, err := lyrahttp.NewServer(lyrahttp.Config{
		Runtime:         &fakeRuntime{},
		Addr:            ":0",
		ServerInfo:      protocol.ServerInfo{Name: "lyra-test", Version: "0.0.0"},
		ProtocolVersion: testProtocolVersion,
		Capabilities:    protocol.ServerCapabilities{},
		AgentDocsLister: func(_ context.Context) []lyrahttp.AgentDocInfo {
			return []lyrahttp.AgentDocInfo{
				{Path: "/proj/AGENTS.md", Bytes: 120},
				{Path: "/proj/pkg/AGENTS.md", Bytes: 42},
			}
		},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/info")
	if err != nil {
		t.Fatalf("get info: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		AgentDocs []struct {
			Path  string `json:"path"`
			Bytes int    `json:"bytes"`
		} `json:"agentDocs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.AgentDocs) != 2 {
		t.Fatalf("agentDocs len = %d, want 2", len(body.AgentDocs))
	}
	if body.AgentDocs[0].Path != "/proj/AGENTS.md" || body.AgentDocs[0].Bytes != 120 {
		t.Fatalf("agentDocs[0] = %+v", body.AgentDocs[0])
	}
	if body.AgentDocs[1].Path != "/proj/pkg/AGENTS.md" || body.AgentDocs[1].Bytes != 42 {
		t.Fatalf("agentDocs[1] = %+v", body.AgentDocs[1])
	}
}

// TestInfoOmitsAgentDocsWhenListerNil confirms the field is absent
// when no lister is wired (rather than appearing as null / empty).
func TestInfoOmitsAgentDocsWhenListerNil(t *testing.T) {
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
	if _, present := body["agentDocs"]; present {
		t.Fatalf("agentDocs should be omitted when lister is nil; body = %+v", body)
	}
}

// TestHealthProbePanicIsContained confirms a panicking probe maps
// to unhealthy instead of crashing the whole /v2/health handler.
func TestHealthProbePanicIsContained(t *testing.T) {
	ts := newProbeServer(t,
		lyrahttp.HealthProbe{
			Name: "bad",
			Probe: func(_ context.Context) lyrahttp.HealthCheck {
				panic("boom")
			},
		},
	)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Checks["bad"] != "unhealthy" {
		t.Fatalf("bad check = %q, want unhealthy", body.Checks["bad"])
	}
}
