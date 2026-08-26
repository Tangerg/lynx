package runtimeembedded

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

func TestCLIProfileFeatureVocabularyMatchesRuntimeProtocol(t *testing.T) {
	t.Parallel()

	want := protocol.FeatureKeys()
	gotNames := runtimeprofile.KnownFeatures()
	got := make([]string, len(gotNames))
	for index, name := range gotNames {
		got[index] = string(name)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("CLI features = %v, runtime features = %v", got, want)
	}
}

func TestRuntimeProfileProjectionPreservesCompleteDiscovery(t *testing.T) {
	t.Parallel()

	discovery := compatibleDiscovery()
	discovery.Capabilities.Features = map[string]protocol.FeatureCapability{
		protocol.FeatureMCP: {
			Enabled: true, ClientOptIn: true, RequiredByRunProtocol: true,
		},
	}
	profile, err := projectRuntimeProfile(discovery, &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{
		protocol.FeatureMCP: {Enabled: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Server.Name != "lyra-runtime" || profile.Server.DefaultWorkspace != "/workspace" ||
		profile.Protocol.Version != protocol.ProtocolVersion ||
		len(profile.RunEvents) != len(discovery.Capabilities.RunEvents) ||
		len(profile.RuntimeTopics) != len(discovery.Capabilities.RuntimeTopics) ||
		len(profile.StreamingMethods) != len(discovery.Capabilities.StreamingMethods) {
		t.Fatalf("runtime profile = %+v", profile)
	}
	feature := profile.Features[runtimeprofile.FeatureMCP]
	if !feature.Enabled || !feature.ClientOptIn ||
		!feature.ClientRequested || !feature.RequiredByRunProtocol || !feature.Available() {
		t.Fatalf("MCP profile = %+v", feature)
	}
	limits := profile.Limits
	if limits.MaxConcurrentRuns != 4 || limits.IdempotencyRetentionSeconds != 600 ||
		limits.IdempotencyNamespace != "idp_test" ||
		limits.RunReplay.MaxEvents != 1024 || limits.RunReplay.MaxBytes != 1<<20 ||
		limits.MCPAuthorizationRetentionSeconds != 600 ||
		limits.RuntimeSubscription.MaxTopics != 32 || limits.RuntimeSubscription.MaxWatches != 32 {
		t.Fatalf("runtime limits = %+v", limits)
	}
}

func TestServicesReturnOwnedProfilesWithoutForkingCapabilityPolicy(t *testing.T) {
	t.Parallel()

	profile, err := projectRuntimeProfile(compatibleDiscovery(), nil)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{profile: profile}
	first := runtime.services()
	second := runtime.services()
	if first.RuntimeProfile == nil || second.RuntimeProfile == nil || first.RuntimeProfile == second.RuntimeProfile {
		t.Fatalf("services profiles = (%p, %p)", first.RuntimeProfile, second.RuntimeProfile)
	}
	first.RuntimeProfile.RuntimeTopics[0] = "mutated"
	if runtime.profile.RuntimeTopics[0] == "mutated" || second.RuntimeProfile.RuntimeTopics[0] == "mutated" {
		t.Fatal("service profile mutation crossed an ownership boundary")
	}
}
