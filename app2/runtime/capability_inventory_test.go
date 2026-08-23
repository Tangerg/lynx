package runtime_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/operation"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestPublishedCapabilityInventoryIsCompleteAndUnique(t *testing.T) {
	methods := operation.Contract().Metas()
	if len(methods) != 89 {
		t.Fatalf("operation count = %d, want 89", len(methods))
	}
	methodNames := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		if method.Name == "" {
			t.Fatal("operation has an empty name")
		}
		if _, duplicate := methodNames[method.Name]; duplicate {
			t.Fatalf("operation %q is published twice", method.Name)
		}
		methodNames[method.Name] = struct{}{}
	}

	runEvents := protocol.RunEventTypes()
	if len(runEvents) != 7 {
		t.Fatalf("Run event variant count = %d, want 7", len(runEvents))
	}
	assertUniqueStrings(t, "Run event", runEvents)

	runtimeTopics := protocol.RuntimeTopics()
	if len(runtimeTopics) != 15 {
		t.Fatalf("subscribable Runtime topic count = %d, want 15", len(runtimeTopics))
	}
	assertUniqueStrings(t, "Runtime topic", runtimeTopics)
	for _, topic := range runtimeTopics {
		if protocol.RuntimeEventType(topic) == protocol.RuntimeResync {
			t.Fatal("resync must remain a server-originated event, not a subscribable topic")
		}
	}
	// The event union is the 15 change topics plus the non-subscribable resync
	// recovery frame.
	if variants := len(runtimeTopics) + 1; variants != 16 {
		t.Fatalf("Runtime event variant count = %d, want 16", variants)
	}

	expectedProbes := map[string]struct{}{
		http.MethodGet + " " + httptransport.PathInfo:      {},
		http.MethodGet + " " + httptransport.PathLiveness:  {},
		http.MethodGet + " " + httptransport.PathReadiness: {},
	}
	for _, endpoint := range httptransport.Contract() {
		if endpoint.Path == httptransport.PathRPC {
			continue
		}
		key := endpoint.Method + " " + endpoint.Path
		if _, expected := expectedProbes[key]; !expected {
			t.Fatalf("unexpected public sidecar endpoint %q", key)
		}
		delete(expectedProbes, key)
	}
	if len(expectedProbes) != 0 {
		t.Fatalf("missing public sidecar endpoints: %v", expectedProbes)
	}
}

func assertUniqueStrings[T ~string](t *testing.T, kind string, values []T) {
	t.Helper()
	seen := make(map[T]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			t.Fatalf("%s has an empty value", kind)
		}
		if _, duplicate := seen[value]; duplicate {
			t.Fatalf("%s %q is published twice", kind, value)
		}
		seen[value] = struct{}{}
	}
}
