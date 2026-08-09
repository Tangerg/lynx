package runs

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func TestPendingValidateRequiresOneCanonicalConnectedTree(t *testing.T) {
	pending := validTreePending()
	if err := pending.Validate(); err != nil {
		t.Fatalf("Validate canonical tree: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Pending)
		want   string
	}{
		{
			name: "non canonical continuation order",
			mutate: func(p *Pending) {
				p.Continuations[0], p.Continuations[1] = p.Continuations[1], p.Continuations[0]
			},
			want: "canonical postorder",
		},
		{
			name: "duplicate opaque executor member binding",
			mutate: func(p *Pending) {
				p.Continuations[0].MemberID = p.Continuations[1].MemberID
			},
			want: "duplicate continuation member",
		},
		{
			name: "disconnected Run",
			mutate: func(p *Pending) {
				p.Continuations[1].Lineage.ParentRunID = "run_b"
				p.Continuations[2].Lineage.ParentRunID = "run_a"
			},
			want: "cycle",
		},
		{
			name: "binding order differs from interrupt order",
			mutate: func(p *Pending) {
				p.Bindings[0], p.Bindings[1] = p.Bindings[1], p.Bindings[0]
			},
			want: "canonical interrupt order",
		},
		{
			name: "pending identity is not canonical",
			mutate: func(p *Pending) {
				p.ExecutorID = " turn_1"
			},
			want: "pending executor ID has surrounding whitespace",
		},
		{
			name: "continuation identity is not canonical",
			mutate: func(p *Pending) {
				p.Continuations[0].MemberID += " "
			},
			want: "member id has surrounding whitespace",
		},
		{
			name: "input request identity is not canonical",
			mutate: func(p *Pending) {
				p.Bindings[0].RequestID += " "
			},
			want: "input request id has surrounding whitespace",
		},
		{
			name: "interrupt identity is not canonical",
			mutate: func(p *Pending) {
				p.Interrupts[0].ItemID += " "
			},
			want: "item id has surrounding whitespace",
		},
		{
			name: "interrupt item occurrence is missing",
			mutate: func(p *Pending) {
				p.Interrupts[0].ItemOccurredAt = time.Time{}
			},
			want: "item occurrence time is required",
		},
		{
			name: "drained tool item occurrence is missing",
			mutate: func(p *Pending) {
				p.Continuations[0].DrainedTools = []DrainedTool{{
					ItemID: "item_open", CallID: "call_open", Name: "shell", Arguments: "{}",
				}}
			},
			want: "item occurrence time is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := validTreePending()
			test.mutate(&candidate)
			err := candidate.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestPendingEqualUsesLogicalDurableValue(t *testing.T) {
	left := validTreePending()
	right := left
	right.CreatedAt = right.CreatedAt.In(time.FixedZone("equal-instant", 8*60*60))
	right.Continuations = slices.Clone(right.Continuations)
	right.Continuations[0].DrainedTools = []DrainedTool{}
	right.Continuations[0].CommittedTools = []CommittedTool{}
	if !left.Equal(right) {
		t.Fatal("Equal rejected equivalent time and empty collection representations")
	}
	right.ExecutorID = "turn_2"
	if left.Equal(right) {
		t.Fatal("Equal accepted a different executor identity")
	}
}

func validTreePending() Pending {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	return Pending{
		RootRunID:  "run_root",
		SessionID:  "session_1",
		ExecutorID: "turn_1",
		Capabilities: run.Capabilities{
			ChildRuns:      true,
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Interrupts: []transcript.Interrupt{
			{
				ItemID: "item_grandchild", ItemOccurredAt: createdAt,
				RunID: "run_grandchild",
				Kind:  interrupt.Approval,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "shell"}, Risk: "medium",
				},
			},
			{
				ItemID: "item_b", ItemOccurredAt: createdAt,
				RunID: "run_b",
				Kind:  interrupt.Approval,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "write"}, Risk: "medium",
				},
			},
		},
		Bindings: []InterruptBinding{
			{InterruptItemID: "item_grandchild", MemberID: "member_grandchild", RequestID: "request_grandchild"},
			{InterruptItemID: "item_b", MemberID: "member_b", RequestID: "request_b"},
		},
		Continuations: []Continuation{
			{
				RunID:    "run_grandchild",
				MemberID: "member_grandchild",
				Lineage: run.Lineage{
					SpawnedByItemID: "item_spawn_grandchild",
					ParentRunID:     "run_a",
					RootRunID:       "run_root",
				},
				RunCreatedAt: createdAt,
			},
			{
				RunID:    "run_a",
				MemberID: "member_a",
				Lineage: run.Lineage{
					SpawnedByItemID: "item_spawn_a",
					ParentRunID:     "run_root",
					RootRunID:       "run_root",
				},
				RunCreatedAt: createdAt,
			},
			{
				RunID:    "run_b",
				MemberID: "member_b",
				Lineage: run.Lineage{
					SpawnedByItemID: "item_spawn_b",
					ParentRunID:     "run_root",
					RootRunID:       "run_root",
				},
				RunCreatedAt: createdAt,
			},
			{
				RunID:        "run_root",
				MemberID:     "member_root",
				RunCreatedAt: createdAt,
			},
		},
		CreatedAt: createdAt,
	}
}
