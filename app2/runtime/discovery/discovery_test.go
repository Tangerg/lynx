package discovery_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/discovery"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestDiscoverPublishesOnlyImplementedCompositionFacts(t *testing.T) {
	t.Parallel()

	service, err := discovery.New(discovery.Config{
		ServerInfo: protocol.ServerInfo{
			InstanceID:       "ins_test",
			Name:             "lyra-runtime",
			Version:          "dev",
			DefaultWorkspace: protocol.WorkspaceRef{Path: "/workspace"},
			Home:             "/home/test",
		},
		IdempotencyNamespace: "idp_test",
	})
	if err != nil {
		t.Fatalf("discovery.New() error = %v", err)
	}

	response, err := service.Discover(t.Context())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if response.ProtocolVersion != protocol.ProtocolVersion {
		t.Fatalf("protocolVersion = %q", response.ProtocolVersion)
	}
	if len(response.Capabilities.RunEvents) != 0 || len(response.Capabilities.RuntimeTopics) != 0 || len(response.Capabilities.StreamingMethods) != 0 {
		t.Fatalf("unimplemented capabilities were advertised: %+v", response.Capabilities)
	}
	for key, feature := range response.Capabilities.Features {
		if feature.Enabled {
			t.Errorf("unimplemented feature %q was enabled", key)
		}
	}
	if got := response.Capabilities.Limits.Idempotency.Namespace; got != "idp_test" {
		t.Fatalf("idempotency namespace = %q", got)
	}
	if err := response.Validate(); err != nil {
		t.Fatalf("DiscoverResponse.Validate() error = %v", err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, field := range [][]byte{[]byte(`"runEvents":[]`), []byte(`"runtimeTopics":[]`), []byte(`"streamingMethods":[]`)} {
		if !bytes.Contains(encoded, field) {
			t.Fatalf("discovery encoded an empty list as null: %s", encoded)
		}
	}
}

func TestDiscoverRejectsUnknownFeatureComposition(t *testing.T) {
	t.Parallel()

	_, err := discovery.New(discovery.Config{
		ServerInfo: protocol.ServerInfo{
			InstanceID:       "ins_test",
			Name:             "lyra-runtime",
			Version:          "dev",
			DefaultWorkspace: protocol.WorkspaceRef{Path: "/workspace"},
			Home:             "/home/test",
		},
		IdempotencyNamespace: "idp_test",
		EnabledFeatures:      map[string]bool{"futureGuess": true},
	})
	if err == nil {
		t.Fatal("discovery.New() accepted an unpublished feature")
	}
}
