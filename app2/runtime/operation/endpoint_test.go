package operation_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/discovery"
	"github.com/Tangerg/lynx/app2/runtime/operation"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestEndpointInvokesTypedDiscovery(t *testing.T) {
	t.Parallel()

	service := newDiscovery(t)
	endpoint, err := operation.New(service, t.Context())
	if err != nil {
		t.Fatalf("operation.New() error = %v", err)
	}

	response, err := operation.Call[struct{}, protocol.DiscoverResponse](
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

	endpoint, err := operation.New(newDiscovery(t), t.Context())
	if err != nil {
		t.Fatalf("operation.New() error = %v", err)
	}

	for _, version := range []string{"2026-07-19", "future"} {
		_, err := operation.Call[struct{}, protocol.DiscoverResponse](
			t.Context(), endpoint, "runtime.discover", struct{}{},
			operation.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: version}},
		)
		if !errors.Is(err, protocol.ErrInvalidProtocolVersion) {
			t.Fatalf("version %q error = %v", version, err)
		}
	}

	if _, err := operation.Call[struct{}, protocol.DiscoverResponse](
		t.Context(), endpoint, "runtime.discover", struct{}{}, operation.Options{},
	); err != nil {
		t.Fatalf("Call() without metadata error = %v", err)
	}
}

func TestContractOwnsOnlyImplementedOperation(t *testing.T) {
	t.Parallel()

	metas := operation.Contract().Metas()
	if len(metas) != 1 || metas[0].Name != "runtime.discover" {
		t.Fatalf("contract methods = %+v", metas)
	}
	if metas[0].Operation != operation.Query || metas[0].Kind != operation.Unary {
		t.Fatalf("runtime.discover metadata = %+v", metas[0])
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
