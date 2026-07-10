package http

import (
	"context"
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
type HealthProbe struct {
	Name  string
	Probe func(ctx context.Context) HealthCheck
}

// healthBudget caps how long /v2/health waits for probes. Probes
// share the budget — a slow downstream doesn't penalize the others.
// 2s matches the typical k8s liveness probe timeout default.
const healthBudget = 2 * time.Second

// runHealthProbes runs every probe in parallel under a shared
// timeout and aggregates worst-of. Panics inside a probe map to
// unhealthy so a misbehaving probe can't crash /v2/health.
func runHealthProbes(ctx context.Context, probes []HealthProbe) (HealthStatus, map[string]HealthStatus) {
	return runHealthProbesWithBudget(ctx, probes, healthBudget)
}

func runHealthProbesWithBudget(
	ctx context.Context,
	probes []HealthProbe,
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
		go func() {
			result := HealthCheck{}
			defer func() {
				if r := recover(); r != nil {
					result = HealthCheck{Status: HealthUnhealthy, Detail: "probe panic"}
				}
				completed <- probeResult{index: i, check: result}
			}()
			result = p.Probe(ctx)
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
		status := r.Status
		if status == "" {
			status = HealthUnhealthy
		}
		checks[probes[i].Name] = status
		overall = worseHealth(overall, status)
	}
	return overall, checks
}

// worseHealth picks the worst of two statuses: ok < degraded < unhealthy.
func worseHealth(a, b HealthStatus) HealthStatus {
	if healthRank(b) > healthRank(a) {
		return b
	}
	return a
}

func healthRank(s HealthStatus) int {
	switch s {
	case HealthUnhealthy:
		return 2
	case HealthDegraded:
		return 1
	default:
		return 0
	}
}
