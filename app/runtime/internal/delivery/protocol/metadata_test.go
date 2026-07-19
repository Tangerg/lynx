package protocol

import (
	"context"
	"testing"
)

func TestRequestMetaContextOwnsSnapshot(t *testing.T) {
	info := &ClientInfo{Name: "before"}
	caps := &ClientCapabilities{
		Events:         []StreamEventType{StreamSegmentStarted},
		InterruptTypes: []InterruptType{"approval"},
		Features: map[string]FeaturePreference{
			"nested": {Enabled: true},
		},
	}
	ctx := WithRequestMeta(context.Background(), RequestMeta{ClientInfo: info, ClientCapabilities: caps})
	info.Name = "after"
	caps.Events[0] = StreamSegmentFinished
	caps.InterruptTypes[0] = "after"
	caps.Features["nested"] = FeaturePreference{Enabled: false}

	first, ok := RequestMetaFrom(ctx)
	if !ok || first.ClientInfo.Name != "before" || first.ClientCapabilities.Events[0] != StreamSegmentStarted || first.ClientCapabilities.InterruptTypes[0] != "approval" {
		t.Fatalf("stored metadata retained caller state: %+v", first)
	}
	if !first.ClientCapabilities.Features["nested"].Enabled {
		t.Fatalf("nested feature retained caller state: %+v", first.ClientCapabilities.Features)
	}
	first.ClientCapabilities.Events[0] = StreamSegmentFinished
	second, _ := RequestMetaFrom(ctx)
	if second.ClientCapabilities.Events[0] != StreamSegmentStarted {
		t.Fatal("RequestMetaFrom exposed context-owned backing storage")
	}
}
