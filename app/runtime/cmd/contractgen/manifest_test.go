package main

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestManifestPublishesToolsetPresentationContracts(t *testing.T) {
	registry, shapes := operation.Contract(), dispatch.WireShapes()
	generated := build(walkWireTypes(registry, shapes))

	want := make(map[string]string)
	for _, contract := range toolset.PresentationContracts() {
		want[contract.ToolName] = "schema.json#/$defs/" + defName(contract.ResultType)
	}
	got := make(map[string]string)
	for _, result := range generated.ResultPresentations {
		if _, exists := got[result.ToolName]; exists {
			t.Fatalf("tool result presentation %q is published twice", result.ToolName)
		}
		got[result.ToolName] = result.Schema.Ref
	}
	if len(got) != len(want) {
		t.Fatalf("published result presentations = %v, want %v", got, want)
	}
	for toolName, wantRef := range want {
		if gotRef := got[toolName]; gotRef != wantRef {
			t.Errorf("result presentation %q schema = %q, want %q", toolName, gotRef, wantRef)
		}
	}
}

func TestGeneratedMethodErrorsIncludeStaticCapabilityRefusals(t *testing.T) {
	t.Parallel()

	manifest := build(newSchemaSet(dispatch.WireShapes()))
	for _, methodName := range []string{"runs.list", "items.list"} {
		index := slices.IndexFunc(manifest.Methods, func(method methodEntry) bool {
			return method.Name == methodName
		})
		if index < 0 {
			t.Fatalf("%s is absent from the manifest", methodName)
		}
		if !slices.Contains(
			manifest.Methods[index].Errors,
			protocol.ErrCapabilityNotNeg.Error(),
		) {
			t.Fatalf("%s errors = %v, want capability_not_negotiated", methodName, manifest.Methods[index].Errors)
		}
	}

	capabilityIndex := slices.IndexFunc(manifest.Errors.Types, func(problem errorEntry) bool {
		return problem.Type == protocol.ErrCapabilityNotNeg.Error()
	})
	if capabilityIndex < 0 {
		t.Fatal("capability_not_negotiated is absent from the error registry")
	}
	for _, methodName := range []string{"runs.list", "items.list"} {
		if !slices.Contains(manifest.Errors.Types[capabilityIndex].Methods, methodName) {
			t.Fatalf(
				"capability_not_negotiated methods = %v, want %s",
				manifest.Errors.Types[capabilityIndex].Methods,
				methodName,
			)
		}
	}
}

func TestGeneratedMethodsPublishDerivedPagination(t *testing.T) {
	t.Parallel()

	manifest := build(newSchemaSet(dispatch.WireShapes()))
	for _, test := range []struct {
		method string
		want   string
	}{
		{method: "sessions.list", want: "cursor"},
		{method: "items.list", want: "cursor"},
		{method: "sessions.get", want: "none"},
	} {
		index := slices.IndexFunc(manifest.Methods, func(method methodEntry) bool {
			return method.Name == test.method
		})
		if index < 0 {
			t.Fatalf("%s is absent from the manifest", test.method)
		}
		if got := manifest.Methods[index].Pagination; got != test.want {
			t.Errorf("%s pagination = %q, want %q", test.method, got, test.want)
		}
	}
}

func TestGeneratedMethodsPublishMaterializedQueryFacts(t *testing.T) {
	t.Parallel()

	manifest := build(newSchemaSet(dispatch.WireShapes()))
	index := slices.IndexFunc(manifest.Methods, func(method methodEntry) bool {
		return method.Name == "sessions.snapshot"
	})
	if index < 0 {
		t.Fatal("sessions.snapshot is absent from the manifest")
	}
	want := []string{"items.list", "runs.list", "interrupts.list", "plan.get", "goals.get"}
	if got := manifest.Methods[index].Materializes; !slices.Equal(got, want) {
		t.Fatalf("sessions.snapshot materializes = %v, want %v", got, want)
	}
}

func TestManifestPublishesImplementedHTTPEndpoints(t *testing.T) {
	t.Parallel()

	generated := build(walkWireTypes(operation.Contract(), dispatch.WireShapes()))
	want := map[string]struct {
		method   string
		path     string
		response string
	}{
		"rpc":       {method: "POST", path: "/v2/rpc"},
		"info":      {method: "GET", path: "/v2/info", response: "schema.json#/$defs/RuntimeInfo"},
		"liveness":  {method: "GET", path: "/v2/health/live", response: "schema.json#/$defs/LivenessStatus"},
		"readiness": {method: "GET", path: "/v2/health/ready", response: "schema.json#/$defs/ReadinessStatus"},
	}
	if len(generated.HTTPEndpoints) != len(want) {
		t.Fatalf("HTTP endpoints = %d, want %d", len(generated.HTTPEndpoints), len(want))
	}
	for _, endpoint := range generated.HTTPEndpoints {
		expected, ok := want[endpoint.Name]
		if !ok {
			t.Errorf("unexpected HTTP endpoint %q", endpoint.Name)
			continue
		}
		if endpoint.Method != expected.method || endpoint.Path != expected.path {
			t.Errorf("%s = %s %s, want %s %s", endpoint.Name, endpoint.Method, endpoint.Path, expected.method, expected.path)
		}
		response := ""
		if endpoint.Response != nil {
			response = endpoint.Response.Ref
		}
		if response != expected.response {
			t.Errorf("%s response = %q, want %q", endpoint.Name, response, expected.response)
		}
	}
}
