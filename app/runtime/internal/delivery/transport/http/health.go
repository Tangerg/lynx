package http

import (
	"context"
	"sync"
	"time"
)

// HealthStatus is the per-check / aggregate status. Per TRANSPORT §12.1,
// `"ok"` maps to HTTP 200; everything else maps to 503.
type HealthStatus string

const (
	HealthOK        HealthStatus = "ok"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
)

// HealthCheck is one probe's result. Detail is optional human-readable
// context for ops; it never lands in the response body — we only
// surface the status keyword.
type HealthCheck struct {
	Status HealthStatus
	Detail string
}

// HealthProbe lets the runtime contribute a labeled liveness check.
// Name is the key under `checks` in the response and should be short
// + stable ("runtime", "storage", "providers").
//
// Probe should honor ctx. The transport still limits each configured probe to
// one in-flight invocation, so an implementation that ignores cancellation can
// strand at most one goroutine instead of one goroutine per health request.
type HealthProbe struct {
	Name  string
	Probe func(ctx context.Context) HealthCheck
}

// healthProbeRunner owns the one in-flight invocation allowed for a configured
// probe. Concurrent health requests share that invocation and independently
// stop waiting when their own request budget expires.
type healthProbeRunner struct {
	name  string
	probe func(context.Context) HealthCheck

	mu      sync.Mutex
	current *healthProbeInvocation
}

type healthProbeInvocation struct {
	done  chan struct{}
	check HealthCheck
}

func newHealthProbeRunners(probes []HealthProbe) []*healthProbeRunner {
	runners := make([]*healthProbeRunner, len(probes))
	for i, probe := range probes {
		runners[i] = &healthProbeRunner{name: probe.Name, probe: probe.Probe}
	}
	return runners
}

func (r *healthProbeRunner) start(ctx context.Context, budget time.Duration) *healthProbeInvocation {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil {
		return r.current
	}
	invocation := &healthProbeInvocation{done: make(chan struct{})}
	r.current = invocation
	// The caller owns only its wait. The runner owns the shared invocation and
	// gives it an independent budget while preserving request-scoped values.
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
	go r.invoke(probeCtx, cancel, invocation)
	return invocation
}

func (r *healthProbeRunner) invoke(
	ctx context.Context,
	cancel context.CancelFunc,
	invocation *healthProbeInvocation,
) {
	defer cancel()
	result := HealthCheck{}
	defer func() {
		if recover() != nil {
			result = HealthCheck{Status: HealthUnhealthy, Detail: "probe panic"}
		}
		invocation.check = result
		r.mu.Lock()
		if r.current == invocation {
			r.current = nil
		}
		r.mu.Unlock()
		close(invocation.done)
	}()
	result = r.probe(ctx)
}

// healthBudget caps how long /v2/health waits for probes. Probes
// share the budget — a slow downstream doesn't penalize the others.
// 2s matches the typical k8s liveness probe timeout default.
const healthBudget = 2 * time.Second

// runHealthProbes runs every probe in parallel under a shared
// timeout and aggregates worst-of. Panics inside a probe map to
// unhealthy so a misbehaving probe can't crash /v2/health.
func runHealthProbes(ctx context.Context, probes []*healthProbeRunner) (HealthStatus, map[string]HealthStatus) {
	return runHealthProbesWithBudget(ctx, probes, healthBudget)
}

func runHealthProbesWithBudget(
	ctx context.Context,
	probes []*healthProbeRunner,
	budget time.Duration,
) (HealthStatus, map[string]HealthStatus) {
	if len(probes) == 0 {
		return HealthOK, nil
	}

	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	type probeResult struct {
		index int
		check HealthCheck
	}
	completed := make(chan probeResult, len(probes))
	results := make([]HealthCheck, len(probes))
	for i, p := range probes {
		invocation := p.start(ctx, budget)
		go func() {
			select {
			case <-invocation.done:
				completed <- probeResult{index: i, check: invocation.check}
			case <-ctx.Done():
			}
		}()
	}
	for range probes {
		select {
		case result := <-completed:
			results[result.index] = result.check
		case <-ctx.Done():
			goto aggregate
		}
	}

aggregate:
	checks := make(map[string]HealthStatus, len(probes))
	overall := HealthOK
	for i, r := range results {
		status := normalizedHealth(r.Status)
		checks[probes[i].name] = status
		overall = worseHealth(overall, status)
	}
	return overall, checks
}

func normalizedHealth(status HealthStatus) HealthStatus {
	switch status {
	case HealthOK, HealthDegraded, HealthUnhealthy:
		return status
	default:
		return HealthUnhealthy
	}
}

// worseHealth picks the worst of two statuses: ok < degraded < unhealthy.
func worseHealth(a, b HealthStatus) HealthStatus {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

func (s HealthStatus) rank() int {
	switch s {
	case HealthUnhealthy:
		return 2
	case HealthDegraded:
		return 1
	default:
		return 0
	}
}
