package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

func TestRuntimeMemoryUnavailable(t *testing.T) {
	rt := &Runtime{}
	ctx := context.Background()

	if _, err := rt.ListMemoryEntries(ctx, "/repo"); !errors.Is(err, ErrMemoryUnavailable) {
		t.Fatalf("ListMemoryEntries err = %v, want ErrMemoryUnavailable", err)
	}
	if _, err := rt.GetMemory(ctx, knowledge.ScopeProject, "/repo"); !errors.Is(err, ErrMemoryUnavailable) {
		t.Fatalf("GetMemory err = %v, want ErrMemoryUnavailable", err)
	}
	if err := rt.UpdateMemory(ctx, knowledge.ScopeUser, "", "prefs"); !errors.Is(err, ErrMemoryUnavailable) {
		t.Fatalf("UpdateMemory err = %v, want ErrMemoryUnavailable", err)
	}
}
