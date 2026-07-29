package protocol

import (
	"context"
	"testing"
)

func TestRequestMetaContextOwnsSnapshot(t *testing.T) {
	info := &ClientInfo{Name: "before"}
	caps := &ClientCapabilities{
		InterruptTypes:          []InterruptType{"approval"},
		ExcludedEphemeralEvents: []StreamEventType{StreamItemDelta},
		Features: map[string]FeaturePreference{
			"nested": {Enabled: true},
		},
	}
	ctx := WithRequestMeta(context.Background(), RequestMeta{ClientInfo: info, ClientCapabilities: caps})
	info.Name = "after"
	caps.InterruptTypes[0] = "after"
	caps.ExcludedEphemeralEvents[0] = StreamSegmentProgress
	caps.Features["nested"] = FeaturePreference{Enabled: false}

	first, ok := RequestMetaFrom(ctx)
	if !ok || first.ClientInfo.Name != "before" || first.ClientCapabilities.InterruptTypes[0] != "approval" {
		t.Fatalf("stored metadata retained caller state: %+v", first)
	}
	if first.ClientCapabilities.ExcludedEphemeralEvents[0] != StreamItemDelta {
		t.Fatalf("exclusion list retained caller state: %+v", first.ClientCapabilities)
	}
	if !first.ClientCapabilities.Features["nested"].Enabled {
		t.Fatalf("nested feature retained caller state: %+v", first.ClientCapabilities.Features)
	}
	first.ClientCapabilities.InterruptTypes[0] = "after"
	second, _ := RequestMetaFrom(ctx)
	if second.ClientCapabilities.InterruptTypes[0] != "approval" {
		t.Fatal("RequestMetaFrom exposed context-owned backing storage")
	}
}
