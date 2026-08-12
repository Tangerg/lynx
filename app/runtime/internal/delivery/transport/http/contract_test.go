package http

import "testing"

func TestHTTPContractHasUniqueCompleteEndpoints(t *testing.T) {
	t.Parallel()

	names := make(map[string]bool)
	bindings := make(map[string]bool)
	for index, endpoint := range Contract().Endpoints {
		if endpoint.Name == "" || endpoint.Method == "" || endpoint.Path == "" {
			t.Fatalf("incomplete endpoint: %+v", endpoint)
		}
		if names[endpoint.Name] {
			t.Errorf("duplicate endpoint name %q", endpoint.Name)
		}
		names[endpoint.Name] = true
		binding := endpoint.Method + " " + endpoint.Path
		if bindings[binding] {
			t.Errorf("duplicate endpoint binding %q", binding)
		}
		bindings[binding] = true
		if len(endpoint.ResponseStatuses) == 0 {
			t.Errorf("endpoint %q has no response status", endpoint.Name)
		}
		if endpoint.Kind == EndpointKindSidecar && endpoint.ResponseType == nil {
			t.Errorf("sidecar endpoint %q has no response type", endpoint.Name)
		}
		if endpointRegistry.Endpoints[index].handler == nil {
			t.Errorf("endpoint %q has no registered handler", endpoint.Name)
		}
	}
}

func TestHTTPContractReturnsAnIsolatedSnapshot(t *testing.T) {
	t.Parallel()

	first := Contract()
	first.Endpoints[0].ResponseStatuses[0] = 999
	first.Enums[0].Values[0] = "invented"
	second := Contract()
	if second.Endpoints[0].ResponseStatuses[0] == 999 || second.Enums[0].Values[0] == "invented" {
		t.Fatal("Contract exposed mutable registry storage")
	}
}
