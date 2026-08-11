package runtimeembedded

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestRequireCompletePageRejectsUnconsumableResults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		page *protocol.Page[string]
		want string
	}{
		{name: "nil page", want: "nil page"},
		{name: "continuation", page: protocol.NewPageWithCursor([]string{"first"}, "next"), want: "continuation cursor"},
		{name: "complete", page: protocol.NewPage([]string{"first"})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			values, err := requireCompletePage("list values", test.page)
			if test.want == "" {
				if err != nil || len(values) != 1 || values[0] != "first" {
					t.Fatalf("requireCompletePage = (%v, %v)", values, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("requireCompletePage error = %v, want %q", err, test.want)
			}
		})
	}
}

type modelCatalogBindingStub struct {
	providers *protocol.Page[protocol.Provider]
	models    map[string]*protocol.Page[protocol.Model]
}

func (stub modelCatalogBindingStub) ListProviders(context.Context, embedded.CallOptions) (*protocol.Page[protocol.Provider], error) {
	return stub.providers, nil
}

func (stub modelCatalogBindingStub) ListModels(_ context.Context, request protocol.ListModelsRequest, _ embedded.CallOptions) (*protocol.Page[protocol.Model], error) {
	return stub.models[request.Provider], nil
}

func TestModelCatalogRejectsEveryUnconsumableContinuation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		stub modelCatalogBindingStub
		want string
	}{
		{
			name: "providers",
			stub: modelCatalogBindingStub{providers: protocol.NewPageWithCursor([]protocol.Provider{}, "next")},
			want: "list providers",
		},
		{
			name: "models",
			stub: modelCatalogBindingStub{
				providers: protocol.NewPage([]protocol.Provider{{ID: "deepseek"}}),
				models: map[string]*protocol.Page[protocol.Model]{
					"deepseek": protocol.NewPageWithCursor([]protocol.Model{}, "next"),
				},
			},
			want: "list models for deepseek",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := &Runtime{modelCatalog: test.stub, meta: requestMeta("test")}
			_, err := runtime.ListModels(t.Context())
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "continuation cursor") {
				t.Fatalf("ListModels error = %v, want %q continuation failure", err, test.want)
			}
		})
	}
}
