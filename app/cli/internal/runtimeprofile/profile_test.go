package runtimeprofile

import "testing"

func TestProfileOwnsCapabilityCollectionsAndAnswersGates(t *testing.T) {
	t.Parallel()

	profile := validProfile()
	if err := profile.Validate(); err != nil {
		t.Fatal(err)
	}
	if !profile.Supports(FeatureMCP) || profile.Supports(FeatureSchedules) || !profile.SupportsRuntimeTopic("files.changed") {
		t.Fatalf("profile gates = %+v", profile)
	}
	if names := profile.AvailableFeatureNames(); len(names) != 1 || names[0] != "mcp" {
		t.Fatalf("available features = %v", names)
	}
	clone := profile.Clone()
	clone.RunEvents[0] = "mutated"
	clone.RuntimeTopics[0] = "mutated"
	clone.StateSnapshots[0].Key = "mutated"
	clone.StreamingMethods[0] = "mutated"
	clone.Features["mcp"] = Feature{}
	if profile.RunEvents[0] == "mutated" || profile.RuntimeTopics[0] == "mutated" ||
		profile.StateSnapshots[0].Key == "mutated" || profile.StreamingMethods[0] == "mutated" ||
		!profile.Features["mcp"].Enabled {
		t.Fatal("profile clone retained caller-owned collections")
	}
}

func TestProfileRequiresClientAgreementForOptInFeatures(t *testing.T) {
	t.Parallel()

	profile := validProfile()
	profile.Features["subagents"] = Feature{Enabled: true, Stability: Stable, ClientOptIn: true}
	if profile.Supports(FeatureSubagents) {
		t.Fatal("server support bypassed the client opt-in requirement")
	}
	feature := profile.Features["subagents"]
	feature.ClientRequested = true
	profile.Features["subagents"] = feature
	if !profile.Supports(FeatureSubagents) {
		t.Fatal("negotiated opt-in feature was unavailable")
	}
}

func TestProfileRejectsIncompleteIdentityCapabilitiesAndLimits(t *testing.T) {
	t.Parallel()

	tests := []Profile{
		{},
		func() Profile {
			value := validProfile()
			value.RunEvents = append(value.RunEvents, value.RunEvents[0])
			return value
		}(),
		func() Profile { value := validProfile(); value.StateSnapshots[0].RecoveryMethod = ""; return value }(),
		func() Profile {
			value := validProfile()
			value.Features["mcp"] = Feature{Stability: "unknown"}
			return value
		}(),
		func() Profile { value := validProfile(); value.Limits.RunReplay.MaxBytes = 0; return value }(),
	}
	for _, profile := range tests {
		if err := profile.Validate(); err == nil {
			t.Fatalf("Validate accepted invalid profile: %+v", profile)
		}
	}
}

func validProfile() Profile {
	return Profile{
		Protocol: Protocol{Current: "2.0", MinSupported: "2.0"},
		Server: Server{
			Name: "lyra-runtime", Version: "dev", DefaultWorkspace: "/workspace", Home: "/home/test",
		},
		RunEvents:        []string{"segment.started"},
		RuntimeTopics:    []string{"files.changed"},
		StateSnapshots:   []Snapshot{{Key: "plan", RecoveryMethod: "plan.get", Scope: "session", Writer: "rootRun"}},
		StreamingMethods: []string{"runs.start"},
		Features: map[FeatureName]Feature{
			FeatureMCP:       {Enabled: true, Stability: Stable},
			FeatureSchedules: {Stability: Experimental},
		},
		Limits: Limits{
			MaxConcurrentRuns: 4, IdempotencyRetentionSeconds: 600,
			RunReplay:                        ReplayLimits{Scope: "runtimeInstanceRootSegment", MaxEvents: 1024, MaxBytes: 1 << 20},
			MCPAuthorizationRetentionSeconds: 600,
			RuntimeSubscription:              SubscriptionLimits{MaxTopics: 16, MaxWatches: 32},
		},
	}
}
