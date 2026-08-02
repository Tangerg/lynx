package server

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestCapabilitiesAdvertiseOnlyProducedRunEvents(t *testing.T) {
	t.Parallel()

	caps := capabilitiesFor(featureAvailability{
		memory: true, git: true, fileWatch: true, todos: true,
		goals: true, agentMemory: true, schedules: true, codebase: true,
	}, replayLimitsFrom(runs.NewCoordinator(runs.Dependencies{})), protocol.MCPAuthorizationAttemptLimits{RetentionSeconds: 73})
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
	if !caps.Features["subagents"].Enabled {
		t.Fatalf("produced subagent Run trees were not advertised: %+v", caps.Features)
	}
	if caps.Features["clientTools"].Enabled {
		t.Fatalf("unsupported client tools were advertised: %+v", caps.Features)
	}
	for _, feature := range []string{"todos", "goals", "agentMemory", "schedules", "codebase"} {
		if !caps.Features[feature].Enabled {
			t.Fatalf("wired feature %q was not advertised: %+v", feature, caps.Features)
		}
	}
	if caps.Limits.MaxConcurrentRuns != 0 {
		t.Fatalf("maxConcurrentRuns = %d, want omitted without an enforced process-wide cap", caps.Limits.MaxConcurrentRuns)
	}
	// The replay window is advertised because it is enforced, and the numbers are the
	// enforcer's own: a client told one bound while the runtime evicts by another
	// would choose replay exactly when replay cannot serve it.
	replay := caps.Limits.RunReplay
	if replay.Scope != protocol.ReplayScopeProcessRootSegment {
		t.Fatalf("replay scope = %q, want %q", replay.Scope, protocol.ReplayScopeProcessRootSegment)
	}
	if replay.MaxEvents != runs.DefaultRetention.MaxEvents || replay.MaxBytes != runs.DefaultRetention.MaxBytes {
		t.Fatalf("replay limits = %+v, want the enforced %+v", replay, runs.DefaultRetention)
	}
	if got := caps.Limits.MCPAuthorizationAttempts.RetentionSeconds; got != 73 {
		t.Fatalf("MCP authorization attempt retention = %d, want the enforcing coordinator's 73", got)
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

	caps := capabilitiesFor(
		featureAvailability{},
		replayLimitsFrom(runs.NewCoordinator(runs.Dependencies{})),
		protocol.MCPAuthorizationAttemptLimits{RetentionSeconds: 73},
	)
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
