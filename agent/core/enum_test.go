package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

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
