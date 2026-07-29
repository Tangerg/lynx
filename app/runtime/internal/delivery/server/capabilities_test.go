package server

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestCapabilitiesAdvertiseOnlyProducedRunEvents(t *testing.T) {
	t.Parallel()

	caps := capabilitiesFor(featureAvailability{
		memory: true, git: true, fileWatch: true, todos: true,
		goals: true, agentMemory: true, schedules: true, codebase: true,
	})
	want := []protocol.StreamEventType{
		protocol.StreamSegmentStarted,
		protocol.StreamSegmentProgress,
		protocol.StreamSegmentFinished,
		protocol.StreamItemStarted,
		protocol.StreamItemDelta,
		protocol.StreamItemCompleted,
		protocol.StreamStateSnapshot,
	}
	if !slices.Equal(caps.RunEvents, want) {
		t.Fatalf("events = %v, want %v", caps.RunEvents, want)
	}
	if caps.Features["subagents"].Enabled || caps.Features["clientTools"].Enabled {
		t.Fatalf("unsupported features advertised: %+v", caps.Features)
	}
	for _, feature := range []string{"todos", "goals", "agentMemory", "schedules", "codebase"} {
		if !caps.Features[feature].Enabled {
			t.Fatalf("wired feature %q was not advertised: %+v", feature, caps.Features)
		}
	}
	if caps.Limits.MaxConcurrentRuns != 0 {
		t.Fatalf("maxConcurrentRuns = %d, want omitted without an enforced process-wide cap", caps.Limits.MaxConcurrentRuns)
	}
}

// TestCapabilitiesAdvertiseThePublishedVocabulary pins discovery to
// [protocol.Features].
//
// The features map is open — §9 says a client reads an absent key as off — which
// makes an omission SILENT: a capability this build supports would simply never
// reach the UI, and nothing would say why. Both directions matter, so a key
// advertised but never published is caught too: a client cannot gate on a name it
// has no way to learn.
func TestCapabilitiesAdvertiseThePublishedVocabulary(t *testing.T) {
	t.Parallel()

	caps := capabilitiesFor(featureAvailability{})
	for _, feature := range protocol.FeatureKeys() {
		if _, advertised := caps.Features[feature]; !advertised {
			t.Errorf("protocol publishes %q and discovery advertises no such key", feature)
		}
	}
	for feature := range caps.Features {
		if _, published := protocol.LookupFeature(feature); !published {
			t.Errorf("discovery advertises %q, which protocol does not publish", feature)
		}
	}
}
