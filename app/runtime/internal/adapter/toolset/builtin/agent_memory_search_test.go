package builtin

import (
	"context"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/agentmemory"
)

type agentMemorySearchStub struct{}

func (agentMemorySearchStub) Search(context.Context, string, string, int) ([]agentmemory.Item, error) {
	return nil, nil
}

func TestNewUsesSearchMemoryContract(t *testing.T) {
	tl, err := NewAgentMemorySearch(agentMemorySearchStub{})
	if err != nil {
		t.Fatal(err)
	}
	if got := tl.Definition().Name; got != "search_memory" {
		t.Fatalf("name = %q, want search_memory", got)
	}
	if _, err := tl.Call(t.Context(), `{"query":"naming","limit":21}`); err == nil {
		t.Fatal("expected an error for limit above 20")
	}
	if _, err := tl.Call(t.Context(), `{"query":"naming","unknown":true}`); err == nil {
		t.Fatal("expected an error for an unknown argument")
	}
}
