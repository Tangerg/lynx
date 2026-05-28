package http

import (
	"encoding/json"
	"net/http"
)

// infoEndpoints is the operational route table surfaced under
// /v1/info.endpoints. Pure server-level metadata — no business data
// here (API.md §9.3 反向不变量).
var infoEndpoints = map[string]string{
	"rpc":    "/v1/rpc/{method}",
	"stream": "/v1/rpc/stream",
	"info":   "/v1/info",
	"health": "/v1/health",
}

// handleInfo serves GET /v1/info — a no-auth flat-JSON snapshot of
// server identity + protocol version + advertised capabilities, plus
// operational metadata (serverID / transport / endpoints) so oncall
// has a single place to look during integration.
//
// API.md §9.2: this endpoint deliberately does NOT use the JSON-RPC
// envelope so oncall can curl it and read the result.
func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Lyra-Server", s.serverID)
	w.WriteHeader(http.StatusOK)

	body := map[string]any{
		"serverInfo":      s.info.ServerInfo,
		"protocolVersion": s.info.ProtocolVersion,
		"capabilities":    s.info.Capabilities,
		"serverID":        s.serverID,
		"transport":       "http",
		"endpoints":       infoEndpoints,
	}
	_ = json.NewEncoder(w).Encode(body)
}

// handleHealth serves GET /v1/health — k8s/nginx liveness probe.
// Runs configured HealthProbes in parallel under a shared timeout
// (API.md §9.2). Status mapping: "ok" → 200, "degraded" /
// "unhealthy" → 503. With no probes configured the response is
// `{"status":"ok"}` (current default behaviour preserved).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	overall, checks := runHealthProbes(r.Context(), s.healthProbes)

	status := http.StatusOK
	if overall != HealthOK {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Lyra-Server", s.serverID)
	w.WriteHeader(status)

	body := struct {
		Status string                  `json:"status"`
		Checks map[string]HealthStatus `json:"checks,omitempty"`
	}{
		Status: string(overall),
		Checks: checks,
	}
	_ = json.NewEncoder(w).Encode(body)
}
