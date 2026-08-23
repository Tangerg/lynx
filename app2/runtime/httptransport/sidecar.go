package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type RuntimeServerInfo struct {
	InstanceID string `json:"instanceId"`
	Name       string `json:"name"`
	Version    string `json:"version"`
}

type RuntimeEndpoints struct {
	RPC       string `json:"rpc"`
	Info      string `json:"info"`
	Liveness  string `json:"liveness"`
	Readiness string `json:"readiness"`
}

type RuntimeInfo struct {
	ProtocolVersion string            `json:"protocolVersion"`
	Server          RuntimeServerInfo `json:"server"`
	Transport       TransportKind     `json:"transport"`
	Endpoints       RuntimeEndpoints  `json:"endpoints"`
}

type TransportKind string

const TransportHTTP TransportKind = "http"

type LivenessStatus struct {
	InstanceID string       `json:"instanceId"`
	Status     HealthStatus `json:"status"`
}

type ReadinessStatus struct {
	InstanceID string                  `json:"instanceId"`
	Status     HealthStatus            `json:"status"`
	Checks     map[string]HealthStatus `json:"checks,omitempty"`
}

type HealthStatus string

const (
	HealthOK        HealthStatus = "ok"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
)

type HealthCheck struct {
	Status HealthStatus
	Detail string
}

type HealthProbe struct {
	Name  string
	Check func(context.Context) HealthCheck
}

type probeRunner struct {
	name   string
	check  func(context.Context) HealthCheck
	mu     sync.Mutex
	flight *probeFlight
}

type probeFlight struct {
	done   chan struct{}
	result HealthCheck
}

func newRuntimeInfo(server protocol.ServerInfo) RuntimeInfo {
	return RuntimeInfo{
		ProtocolVersion: protocol.ProtocolVersion,
		Server:          RuntimeServerInfo{InstanceID: server.InstanceID, Name: server.Name, Version: server.Version},
		Transport:       TransportHTTP,
		Endpoints:       RuntimeEndpoints{RPC: PathRPC, Info: PathInfo, Liveness: PathLiveness, Readiness: PathReadiness},
	}
}

func newProbeRunners(probes []HealthProbe) ([]*probeRunner, error) {
	seen := make(map[string]struct{}, len(probes))
	runners := make([]*probeRunner, 0, len(probes))
	for _, probe := range probes {
		if probe.Name == "" || probe.Check == nil {
			return nil, errors.New("httptransport: each health probe needs a name and function")
		}
		if _, exists := seen[probe.Name]; exists {
			return nil, errors.New("httptransport: duplicate health probe " + probe.Name)
		}
		seen[probe.Name] = struct{}{}
		runners = append(runners, &probeRunner{name: probe.Name, check: probe.Check})
	}
	return runners, nil
}

func (runner *probeRunner) observe(ctx context.Context) *probeFlight {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.flight != nil {
		return runner.flight
	}
	flight := &probeFlight{done: make(chan struct{})}
	runner.flight = flight
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), healthBudget)
	go func() {
		defer cancel()
		result := HealthCheck{Status: HealthUnhealthy}
		defer func() {
			if recover() != nil {
				result = HealthCheck{Status: HealthUnhealthy, Detail: "probe panic"}
			}
			flight.result = result
			runner.mu.Lock()
			if runner.flight == flight {
				runner.flight = nil
			}
			runner.mu.Unlock()
			close(flight.done)
		}()
		result = runner.check(probeCtx)
	}()
	return flight
}

func (server *Server) serveInfo(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, server.info)
}

func (server *Server) serveLiveness(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, LivenessStatus{InstanceID: server.info.Server.InstanceID, Status: HealthOK})
}

func (server *Server) serveReadiness(response http.ResponseWriter, request *http.Request) {
	status, checks := server.readiness(request.Context())
	httpStatus := http.StatusOK
	if status != HealthOK {
		httpStatus = http.StatusServiceUnavailable
	}
	writeJSON(response, httpStatus, ReadinessStatus{
		InstanceID: server.info.Server.InstanceID,
		Status:     status,
		Checks:     checks,
	})
}

func (server *Server) readiness(parent context.Context) (HealthStatus, map[string]HealthStatus) {
	if len(server.probes) == 0 {
		return HealthOK, nil
	}
	ctx, cancel := context.WithTimeout(parent, healthBudget)
	defer cancel()
	flights := make([]*probeFlight, len(server.probes))
	for index, runner := range server.probes {
		flights[index] = runner.observe(ctx)
	}
	checks := make(map[string]HealthStatus, len(server.probes))
	overall := HealthOK
	for index, runner := range server.probes {
		status := HealthUnhealthy
		select {
		case <-flights[index].done:
			status = normalizeHealth(flights[index].result.Status)
		case <-ctx.Done():
		}
		checks[runner.name] = status
		if healthRank(status) > healthRank(overall) {
			overall = status
		}
	}
	return overall, checks
}

const healthBudget = 2 * time.Second

func normalizeHealth(status HealthStatus) HealthStatus {
	if status == HealthOK || status == HealthDegraded || status == HealthUnhealthy {
		return status
	}
	return HealthUnhealthy
}

func healthRank(status HealthStatus) int {
	if status == HealthUnhealthy {
		return 2
	}
	if status == HealthDegraded {
		return 1
	}
	return 0
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
