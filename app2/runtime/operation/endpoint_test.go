package operation_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/discovery"
	"github.com/Tangerg/lynx/app2/runtime/idempotency"
	"github.com/Tangerg/lynx/app2/runtime/operation"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestEndpointInvokesTypedDiscovery(t *testing.T) {
	t.Parallel()

	service := newDiscovery(t)
	endpoint, err := newEndpoint(t, service)
	if err != nil {
		t.Fatalf("operation.New() error = %v", err)
	}

	response, err := operation.Call[struct{}, *protocol.DiscoverResponse](
		t.Context(), endpoint, "runtime.discover", struct{}{},
		operation.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}},
	)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if response.ServerInfo.InstanceID != "ins_test" {
		t.Fatalf("instanceId = %q", response.ServerInfo.InstanceID)
	}
}

func TestEndpointRejectsOnlyProvidedMismatchedVersion(t *testing.T) {
	t.Parallel()

	endpoint, err := newEndpoint(t, newDiscovery(t))
	if err != nil {
		t.Fatalf("operation.New() error = %v", err)
	}

	for _, version := range []string{"2026-07-19", "future"} {
		_, err := operation.Call[struct{}, *protocol.DiscoverResponse](
			t.Context(), endpoint, "runtime.discover", struct{}{},
			operation.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: version}},
		)
		if !errors.Is(err, protocol.ErrInvalidProtocolVersion) {
			t.Fatalf("version %q error = %v", version, err)
		}
	}

	if _, err := operation.Call[struct{}, *protocol.DiscoverResponse](
		t.Context(), endpoint, "runtime.discover", struct{}{}, operation.Options{},
	); err != nil {
		t.Fatalf("Call() without metadata error = %v", err)
	}
}

func TestContractOwnsRuntimeDiscoverMetadata(t *testing.T) {
	t.Parallel()

	meta, ok := operation.Contract().Lookup("runtime.discover")
	if !ok {
		t.Fatal("runtime.discover is not registered")
	}
	if meta.Operation != operation.OperationQuery || meta.Kind != operation.KindUnary {
		t.Fatalf("runtime.discover metadata = %+v", meta)
	}
}

func newDiscovery(t *testing.T) *discovery.Service {
	t.Helper()
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
	return service
}

func newEndpoint(t *testing.T, target any) (*operation.Endpoint, error) {
	t.Helper()
	store, err := idempotency.NewMemoryStore(24 * time.Hour)
	if err != nil {
		return nil, err
	}
	return operation.New(target, operation.Config{
		Lifetime: t.Context(), IdempotencyStore: store,
		IdempotencyNamespace: "idp_test",
	})
}
