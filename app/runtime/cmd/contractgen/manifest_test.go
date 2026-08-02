package main

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

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
