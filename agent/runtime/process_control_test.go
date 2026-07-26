package runtime

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestProcessSignalsMergeTerminationByScope(t *testing.T) {
	signals := newProcessSignals()
	signals.queueTermination(core.TerminationScopeAction, "retry action")
	signals.queueTermination(core.TerminationScopeAgent, "stop process")
	signals.queueTermination(core.TerminationScopeAction, "late action")
	signals.queueTermination(core.TerminationScopeAgent, "late process")

	got := signals.drainTerminate()
	if got == nil {
		t.Fatal("drainTerminate() = nil, want signal")
	}
	if got.Scope != core.TerminationScopeAgent || got.Reason != "stop process" {
		t.Fatalf("signal = %#v, want first agent-scoped request", got)
	}
	if got := signals.drainTerminate(); got != nil {
		t.Fatalf("second drainTerminate() = %#v, want nil", got)
	}
}

func TestProcessSignalsPreferAgentTerminationInEveryArrivalOrder(t *testing.T) {
	tests := []struct {
		name  string
		first core.TerminationScope
		last  core.TerminationScope
	}{
		{name: "action then agent", first: core.TerminationScopeAction, last: core.TerminationScopeAgent},
		{name: "agent then action", first: core.TerminationScopeAgent, last: core.TerminationScopeAction},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signals := newProcessSignals()
			signals.queueTermination(test.first, terminationReason(test.first))
			signals.queueTermination(test.last, terminationReason(test.last))

			got := signals.drainTerminate()
			if got == nil || got.Scope != core.TerminationScopeAgent || got.Reason != "stop process" {
				t.Fatalf("signal = %#v, want agent-scoped request", got)
			}
		})
	}
}

func terminationReason(scope core.TerminationScope) string {
	if scope == core.TerminationScopeAgent {
		return "stop process"
	}
	return "retry action"
}
