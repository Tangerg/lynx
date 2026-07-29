package protocol

import (
	"slices"
	"testing"
)

func TestMissingFeatureRequirementsHonorsServerSupportAndClientOptIn(t *testing.T) {
	t.Parallel()

	advertised := map[string]FeatureCapability{
		FeatureMemory: {
			Enabled: true,
		},
		FeatureSubagents: {
			Enabled: true, ClientOptIn: true, RequiredByRunProtocol: true,
		},
	}
	wantSubagents := []CapabilityRequirement{{
		Type: RequirementFeature, Name: FeatureSubagents,
	}}

	if got := MissingFeatureRequirements(advertised, nil, FeatureMemory); len(got) != 0 {
		t.Fatalf("stable server feature gaps = %v, want none", got)
	}
	if got := MissingFeatureRequirements(advertised, nil, FeatureSubagents); !slices.Equal(got, wantSubagents) {
		t.Fatalf("missing client opt-in gaps = %v, want %v", got, wantSubagents)
	}
	client := &ClientCapabilities{Features: map[string]FeaturePreference{
		FeatureSubagents: {Enabled: true},
	}}
	if got := MissingFeatureRequirements(advertised, client, FeatureSubagents); len(got) != 0 {
		t.Fatalf("negotiated subagents gaps = %v, want none", got)
	}
	if got := MissingFeatureRequirements(nil, client, FeatureSubagents); !slices.Equal(got, wantSubagents) {
		t.Fatalf("unsupported server feature gaps = %v, want %v", got, wantSubagents)
	}
}

func TestMissingFeatureRequirementsIsOrderedAndUnique(t *testing.T) {
	t.Parallel()

	got := MissingFeatureRequirements(nil, nil, FeatureSubagents, FeatureMemory, FeatureSubagents)
	want := []CapabilityRequirement{
		{Type: RequirementFeature, Name: FeatureSubagents},
		{Type: RequirementFeature, Name: FeatureMemory},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("requirements = %v, want ordered unique %v", got, want)
	}
}
