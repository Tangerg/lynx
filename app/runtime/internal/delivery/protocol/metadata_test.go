package protocol

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRequestMetaContextOwnsSnapshot(t *testing.T) {
	info := &ClientInfo{Name: "before"}
	caps := &ClientCapabilities{
		InterruptTypes:          []InterruptType{"approval"},
		ExcludedEphemeralEvents: []SuppressibleRunEventType{SuppressibleRunItemDelta},
		Features: map[string]FeaturePreference{
			"nested": {Enabled: true},
		},
	}
	ctx := WithRequestMeta(context.Background(), RequestMeta{ClientInfo: info, ClientCapabilities: caps})
	info.Name = "after"
	caps.InterruptTypes[0] = "after"
	caps.ExcludedEphemeralEvents[0] = SuppressibleRunSegmentProgress
	caps.Features["nested"] = FeaturePreference{Enabled: false}

	first, ok := RequestMetaFrom(ctx)
	if !ok || first.ClientInfo.Name != "before" || first.ClientCapabilities.InterruptTypes[0] != "approval" {
		t.Fatalf("stored metadata retained caller state: %+v", first)
	}
	if first.ClientCapabilities.ExcludedEphemeralEvents[0] != SuppressibleRunItemDelta {
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

func TestClientCapabilitiesValidateExcludedEphemeralEvents(t *testing.T) {
	t.Parallel()

	valid := ClientCapabilities{
		ExcludedEphemeralEvents: []SuppressibleRunEventType{
			SuppressibleRunSegmentProgress,
			SuppressibleRunItemDelta,
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected the complete opt-out set: %v", err)
	}

	for _, event := range []SuppressibleRunEventType{"custom", "item.completed", "vendor.preview"} {
		event := event
		t.Run(string(event), func(t *testing.T) {
			t.Parallel()

			err := (ClientCapabilities{
				ExcludedEphemeralEvents: []SuppressibleRunEventType{event},
			}).Validate()
			if !errors.Is(err, ErrNonSuppressibleRunEvent) {
				t.Fatalf("Validate error = %v, want ErrNonSuppressibleRunEvent", err)
			}
			if !strings.Contains(err.Error(), string(event)) {
				t.Fatalf("Validate error = %q, want offending value %q", err, event)
			}
		})
	}
}
