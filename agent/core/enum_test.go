package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestPlannerTypeString(t *testing.T) {
	if got := core.PlannerGOAP.String(); got != "goap" {
		t.Fatalf("PlannerGOAP.String() = %q, want goap", got)
	}
	if got := core.PlannerHTN.String(); got != "htn" {
		t.Fatalf("PlannerHTN.String() = %q, want htn", got)
	}
	if got := core.PlannerReactive.String(); got != "reactive" {
		t.Fatalf("PlannerReactive.String() = %q, want reactive", got)
	}
	if got := core.PlannerType(99).String(); got != "unknown_planner_type(99)" {
		t.Fatalf("unknown planner string = %q", got)
	}
}

func TestProcessTypeString(t *testing.T) {
	if got := core.ProcessSequential.String(); got != "sequential" {
		t.Fatalf("ProcessSequential.String() = %q, want sequential", got)
	}
	if got := core.ProcessConcurrent.String(); got != "concurrent" {
		t.Fatalf("ProcessConcurrent.String() = %q, want concurrent", got)
	}
	if got := core.ProcessType(99).String(); got != "unknown_process_type(99)" {
		t.Fatalf("unknown process type string = %q", got)
	}
}
