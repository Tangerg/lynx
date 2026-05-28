package http

import (
	"encoding/json"
	"net/http"
)

// handleInfo serves GET /v1/info — a no-auth flat-JSON snapshot of
// server identity + protocol version + advertised capabilities.
//
// API.md §9.2: this endpoint deliberately does NOT use the JSON-RPC
// envelope so oncall can curl it and read the result.
func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Lyra-Server", s.serverID)
	w.WriteHeader(http.StatusOK)

	// Mirror runtime.initialize.result's read-only subset. We
	// intentionally omit clientInfo + the per-handshake fields —
	// sidecar shows what the server is, not what a client negotiated.
	body := map[string]any{
		"serverInfo":      s.info.ServerInfo,
		"protocolVersion": s.info.ProtocolVersion,
		"capabilities":    s.info.Capabilities,
	}
	_ = json.NewEncoder(w).Encode(body)
}

// handleHealth serves GET /v1/health — k8s/nginx liveness probe.
// Returns 200 / "ok" today; future versions can return 503 with a
// "degraded" body when downstream probes fail.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Lyra-Server", s.serverID)

	body := struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks,omitempty"`
	}{
		Status: "ok",
		Checks: nil,
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}
