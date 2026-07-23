package server

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestCapabilitiesAdvertiseOnlyProducedRunEvents(t *testing.T) {
	t.Parallel()

	caps := Capabilities(true, true)
	want := []protocol.StreamEventType{
		protocol.StreamSegmentStarted,
		protocol.StreamSegmentProgress,
		protocol.StreamSegmentFinished,
		protocol.StreamItemStarted,
		protocol.StreamItemDelta,
		protocol.StreamItemCompleted,
		protocol.StreamStateSnapshot,
	}
	if !slices.Equal(caps.Events, want) {
		t.Fatalf("events = %v, want %v", caps.Events, want)
	}
	if caps.Features["subagents"].Enabled || caps.Features["clientTools"].Enabled {
		t.Fatalf("unsupported features advertised: %+v", caps.Features)
	}
	if caps.Limits.MaxConcurrentRuns != 0 {
		t.Fatalf("maxConcurrentRuns = %d, want omitted without an enforced process-wide cap", caps.Limits.MaxConcurrentRuns)
	}
}
