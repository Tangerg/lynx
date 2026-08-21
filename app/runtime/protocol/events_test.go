package protocol

import "testing"

func TestStreamEventReliabilityIsTypeOwned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		event         StreamEvent
		authoritative bool
		replayable    bool
	}{
		{event: StreamEvent{Type: StreamSegmentStarted}, authoritative: true, replayable: true},
		{event: StreamEvent{Type: StreamSegmentProgress}},
		{event: StreamEvent{Type: StreamSegmentFinished}, authoritative: true, replayable: true},
		{event: StreamEvent{Type: StreamItemStarted}, authoritative: true, replayable: true},
		{event: StreamEvent{Type: StreamItemDelta}},
		{event: StreamEvent{Type: StreamItemCompleted}, authoritative: true, replayable: true},
		{event: StreamEvent{Type: StreamPlanUpdated}, authoritative: true, replayable: true},
		{event: StreamEvent{Type: StreamEventType("unknown")}},
	}

	for _, test := range tests {
		t.Run(string(test.event.Type), func(t *testing.T) {
			t.Parallel()
			if got := test.event.Authoritative(); got != test.authoritative {
				t.Errorf("Authoritative() = %t, want %t", got, test.authoritative)
			}
			if got := test.event.Replayable(); got != test.replayable {
				t.Errorf("Replayable() = %t, want %t", got, test.replayable)
			}
		})
	}
}
