package http

import (
	"encoding/json"
	"net/http"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

type RuntimeServerInfo struct {
	InstanceID string `json:"instanceId"`
	Name       string `json:"name"`
	Version    string `json:"version"`
}

type RuntimeInfoEndpoints struct {
	RPC       string `json:"rpc"`
	Info      string `json:"info"`
	Liveness  string `json:"liveness"`
	Readiness string `json:"readiness"`
}

type HTTPTransportKind string

const HTTPTransport HTTPTransportKind = "http"

type RuntimeInfo struct {
	ProtocolVersion string               `json:"protocolVersion"`
	Server          RuntimeServerInfo    `json:"server"`
	Transport       HTTPTransportKind    `json:"transport"`
	Endpoints       RuntimeInfoEndpoints `json:"endpoints"`
}

func newInfoResponse(server protocol.ServerInfo, currentVersion string) RuntimeInfo {
	return RuntimeInfo{
		ProtocolVersion: currentVersion,
		Server: RuntimeServerInfo{
			InstanceID: server.InstanceID,
			Name:       server.Name,
			Version:    server.Version,
		},
		Transport: HTTPTransport,
		Endpoints: RuntimeInfoEndpoints{
			RPC:       endpointPath(endpointRPC),
			Info:      endpointPath(endpointInfo),
			Liveness:  endpointPath(endpointLiveness),
			Readiness: endpointPath(endpointReadiness),
		},
	}
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(s.info); err != nil {
		return
	}
}

type LivenessState string

const LivenessOK LivenessState = "ok"

type LivenessStatus struct {
	InstanceID string        `json:"instanceId"`
	Status     LivenessState `json:"status"`
}

func (s *Server) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(LivenessStatus{
		InstanceID: s.info.Server.InstanceID,
		Status:     LivenessOK,
	}); err != nil {
		return
	}
}

type ReadinessStatus struct {
	InstanceID string                  `json:"instanceId"`
	Status     HealthStatus            `json:"status"`
	Checks     map[string]HealthStatus `json:"checks,omitempty"`
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

	if err := json.NewEncoder(w).Encode(ReadinessStatus{
		InstanceID: s.info.Server.InstanceID,
		Status:     overall,
		Checks:     checks,
	}); err != nil {
		return
	}
}
