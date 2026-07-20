package runtime

import (
	"sync"
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

func TestProcessSignalsPreferAgentTerminationConcurrently(t *testing.T) {
	for range 100 {
		signals := newProcessSignals()
		var callers sync.WaitGroup
		callers.Add(2)
		go func() {
			defer callers.Done()
			signals.queueTermination(core.TerminationScopeAction, "retry action")
		}()
		go func() {
			defer callers.Done()
			signals.queueTermination(core.TerminationScopeAgent, "stop process")
		}()
		callers.Wait()

		got := signals.drainTerminate()
		if got == nil || got.Scope != core.TerminationScopeAgent || got.Reason != "stop process" {
			t.Fatalf("signal = %#v, want agent-scoped request", got)
		}
	}
}
