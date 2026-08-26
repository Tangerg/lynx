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
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestCursorTraversalRejectsDirectAndMultiStepCycles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		initial  string
		sequence []string
		wantMore []bool
		wantErr  bool
	}{
		{name: "complete", sequence: []string{"next", ""}, wantMore: []bool{true, false}},
		{name: "direct cycle", initial: "current", sequence: []string{"current"}, wantErr: true},
		{name: "multi-step cycle", sequence: []string{"first", "second", "first"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			traversal := newCursorTraversal("list values", test.initial)
			for index, next := range test.sequence {
				more, err := traversal.Advance(next)
				if err != nil {
					if !test.wantErr || !strings.Contains(err.Error(), "cyclic continuation cursor") {
						t.Fatalf("Advance(%q) error = %v", next, err)
					}
					requireRuntimeContractViolation(t, err)
					return
				}
				if index < len(test.wantMore) && more != test.wantMore[index] {
					t.Fatalf("Advance(%q) more = %t, want %t", next, more, test.wantMore[index])
				}
			}
			if test.wantErr {
				t.Fatal("cursor cycle was accepted")
			}
		})
	}
}

type modelCatalogBindingStub struct {
	providers *protocol.Page[protocol.Provider]
	models    map[string]*protocol.Page[protocol.Model]
}

func (m modelCatalogBindingStub) ListProviders(context.Context, embedded.CallOptions) (*protocol.Page[protocol.Provider], error) {
	return m.providers, nil
}

func (m modelCatalogBindingStub) ListModels(_ context.Context, request protocol.ListModelsRequest, _ embedded.CallOptions) (*protocol.Page[protocol.Model], error) {
	return m.models[request.Provider], nil
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
