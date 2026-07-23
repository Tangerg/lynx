package todopresentation

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

func TestRenderIncludesWorkflowFields(t *testing.T) {
	got := Render([]todo.Item{{
		Content:       "stabilize subagent hooks",
		Status:        todo.StatusInProgress,
		BlockedReason: "waiting on event payload",
		NextAction:    "read ProcessCreated bindings",
	}})
	for _, want := range []string{"[~] stabilize subagent hooks", "blocked: waiting on event payload", "next: read ProcessCreated bindings"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered todos = %q, missing %q", got, want)
		}
	}
}

func TestRenderUsesStableChecklistMarkers(t *testing.T) {
	items := []todo.Item{
		{Content: "write tests", Status: todo.StatusCompleted},
		{Content: "ship it", Status: todo.StatusInProgress},
		{Content: "celebrate", Status: todo.StatusPending},
	}
	if got, want := Render(items), "[x] write tests\n[~] ship it\n[ ] celebrate\n"; got != want {
		t.Fatalf("rendered todos = %q, want %q", got, want)
	}
	if got := Render(nil); got != "" {
		t.Fatalf("empty todos = %q, want empty", got)
	}
}
