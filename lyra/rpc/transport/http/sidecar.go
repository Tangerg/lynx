package http

import (
	"encoding/json"
	"net/http"
)

// infoEndpoints is the operational route table surfaced under
// /v2/info.endpoints. Pure server-level metadata — no business data
// here (TRANSPORT §16 反向不变量).
var infoEndpoints = map[string]string{
	"rpc":    "/v2/rpc/{method}",
	"stream": "/v2/rpc/stream",
	"info":   "/v2/info",
	"health": "/v2/health",
}

// handleInfo serves GET /v2/info — a no-auth flat-JSON snapshot of
// server identity + protocol version + advertised capabilities, plus
// operational metadata (serverID / transport / endpoints / discovered
// AGENTS.md files) so oncall has a single place to look during
// integration.
//
// TRANSPORT §12.2: this endpoint deliberately does NOT use the JSON-RPC
// envelope so oncall can curl it and read the result.
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Server", s.serverID)
	w.WriteHeader(http.StatusOK)

	body := map[string]any{
		"serverInfo":      s.info.ServerInfo,
		"protocolVersion": s.info.ProtocolVersion,
		"capabilities":    s.info.Capabilities,
		"serverID":        s.serverID,
		"transport":       "http",
		"endpoints":       infoEndpoints,
	}
	if s.agentDocsLister != nil {
		body["agentDocs"] = s.agentDocsLister(r.Context())
	}
	_ = json.NewEncoder(w).Encode(body)
}

// handleHealth serves GET /v2/health — k8s/nginx liveness probe.
// Runs configured HealthProbes in parallel under a shared timeout
// (TRANSPORT §12.1). Status mapping: "ok" → 200, "degraded" /
// "unhealthy" → 503. The contract body is `{ "ok": true }`; `status`
// (worst-of keyword) and `checks` (per-probe detail) are additive ops
// fields the FE ignores.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	overall, checks := runHealthProbes(r.Context(), s.healthProbes)

	status := http.StatusOK
	if overall != HealthOK {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Server", s.serverID)
	w.WriteHeader(status)

	body := struct {
		OK     bool                    `json:"ok"`
		Status string                  `json:"status"`
		Checks map[string]HealthStatus `json:"checks,omitempty"`
	}{
		OK:     overall == HealthOK,
		Status: string(overall),
		Checks: checks,
	}
	_ = json.NewEncoder(w).Encode(body)
}
