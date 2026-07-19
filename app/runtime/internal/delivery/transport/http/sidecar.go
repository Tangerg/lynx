package http

import (
	"encoding/json"
	"net/http"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

const (
	rpcPath       = "/v2/rpc"
	infoPath      = "/v2/info"
	livenessPath  = "/v2/health/live"
	readinessPath = "/v2/health/ready"
)

type publicServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type infoEndpoints struct {
	RPC       string `json:"rpc"`
	Info      string `json:"info"`
	Liveness  string `json:"liveness"`
	Readiness string `json:"readiness"`
}

type infoResponse struct {
	Protocol  protocol.ProtocolRange `json:"protocol"`
	Server    publicServerInfo       `json:"server"`
	Transport string                 `json:"transport"`
	Endpoints infoEndpoints          `json:"endpoints"`
}

func newInfoResponse(server protocol.ServerInfo, currentVersion string) infoResponse {
	return infoResponse{
		Protocol:  protocol.ProtocolRange{Current: currentVersion, MinSupported: protocol.MinProtocolVersion},
		Server:    publicServerInfo{Name: server.Name, Version: server.Version},
		Transport: "http",
		Endpoints: infoEndpoints{
			RPC:       rpcPath,
			Info:      infoPath,
			Liveness:  livenessPath,
			Readiness: readinessPath,
		},
	}
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(s.info)
}

type livenessResponse struct {
	Status string `json:"status"`
}

func (s *Server) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(livenessResponse{Status: "ok"})
}

type readinessResponse struct {
	Status string                  `json:"status"`
	Checks map[string]HealthStatus `json:"checks,omitempty"`
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	overall, checks := runHealthProbes(r.Context(), s.healthProbes)

	status := http.StatusOK
	if overall != HealthOK {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(readinessResponse{Status: string(overall), Checks: checks})
}
