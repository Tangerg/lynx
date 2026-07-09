package todo

import (
	"errors"
	"strings"
	"testing"
)

func TestValidate_WorkflowFields(t *testing.T) {
	prev := []Item{{Content: "done", Status: StatusCompleted}}
	next := []Item{
		{Content: "done", Status: StatusCompleted},
		{Content: "investigate failure", Status: StatusInProgress, BlockedReason: "test fixture missing", NextAction: "create a focused fake"},
	}
	if err := Validate(prev, next); err != nil {
		t.Fatalf("Validate workflow fields: %v", err)
	}

	bad := []Item{{Content: "ship", Status: StatusCompleted, NextAction: "run tests"}}
	if err := Validate(nil, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("completed with next_action err = %v, want ErrInvalid", err)
	}
}

func TestValidate_ContentRequired(t *testing.T) {
	if err := Validate(nil, []Item{{Status: StatusPending}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing content err = %v, want ErrInvalid", err)
	}
}

func TestRender_WorkflowFields(t *testing.T) {
	got := Render([]Item{{
		Content:       "stabilize subagent hooks",
		Status:        StatusInProgress,
		BlockedReason: "waiting on event payload",
		NextAction:    "read ProcessCreated bindings",
	}})
	for _, want := range []string{
		"[~] stabilize subagent hooks",
		"blocked: waiting on event payload",
		"next: read ProcessCreated bindings",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Render = %q, missing %q", got, want)
		}
	}
}
