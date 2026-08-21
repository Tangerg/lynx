package protocol

import "testing"

func TestClientCapabilitiesWireValidationRejectsUnknownEphemeralEvents(t *testing.T) {
	t.Parallel()

	valid := ClientCapabilities{
		ExcludedEphemeralEvents: []SuppressibleRunEventType{
			SuppressibleRunSegmentProgress,
			SuppressibleRunItemDelta,
		},
	}
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected the complete opt-out set: %v", err)
	}

	for _, event := range []SuppressibleRunEventType{"item.completed", "vendor.preview"} {
		t.Run(string(event), func(t *testing.T) {
			t.Parallel()

			err := (ClientCapabilities{
				ExcludedEphemeralEvents: []SuppressibleRunEventType{event},
			}).ValidateWire()
			assertConstraintField(t, err, "ClientCapabilities", "excludedEphemeralEvents[0]")
		})
	}
}
