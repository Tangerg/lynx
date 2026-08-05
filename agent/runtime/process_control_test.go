package runtime

import "testing"

func TestProcessSignalsRetainFirstTerminationReason(t *testing.T) {
	signals := newProcessSignals()
	signals.queueTermination("first cause")
	signals.queueTermination("late cause")

	got := signals.drainTermination()
	if got == nil {
		t.Fatal("drainTermination() = nil, want request")
	}
	if got.reason != "first cause" {
		t.Fatalf("reason = %q, want first cause", got.reason)
	}
	if got := signals.drainTermination(); got != nil {
		t.Fatalf("second drainTermination() = %#v, want nil", got)
	}
}
