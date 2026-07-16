package htn

import "go.opentelemetry.io/otel"

// plannerTracer is the package-level tracer for the HTN planner.
// Tracer name follows the `lynx/agent/planner` namespace shared with
// the GOAP planner — backends can distinguish algorithms by the
// span name (`htn.plan` vs `agent.planner.goap`).
var plannerTracer = otel.Tracer("lynx/agent/planner")
