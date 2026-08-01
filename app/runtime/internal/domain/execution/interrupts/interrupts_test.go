package interrupts

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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
			name: "duplicate opaque process binding",
			mutate: func(p *Pending) {
				p.Continuations[0].ProcessID = p.Continuations[1].ProcessID
			},
			want: "duplicate continuation process",
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
				p.Suspensions[0], p.Suspensions[1] = p.Suspensions[1], p.Suspensions[0]
			},
			want: "canonical interrupt order",
		},
		{
			name: "pending identity is not canonical",
			mutate: func(p *Pending) {
				p.TurnID = " turn_1"
			},
			want: "pending turn id has surrounding whitespace",
		},
		{
			name: "continuation identity is not canonical",
			mutate: func(p *Pending) {
				p.Continuations[0].ProcessID += " "
			},
			want: "process id has surrounding whitespace",
		},
		{
			name: "suspension identity is not canonical",
			mutate: func(p *Pending) {
				p.Suspensions[0].SuspensionID += " "
			},
			want: "suspension id has surrounding whitespace",
		},
		{
			name: "interrupt identity is not canonical",
			mutate: func(p *Pending) {
				p.Interrupts[0].ItemID += " "
			},
			want: "item id has surrounding whitespace",
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

func validTreePending() Pending {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	return Pending{
		RootRunID: "run_root",
		SessionID: "session_1",
		TurnID:    "turn_1",
		ProtocolProfile: execution.RunProtocolProfile{
			ChildRuns:      true,
			InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
		},
		Interrupts: []transcript.Interrupt{
			{
				ItemID: "item_grandchild",
				RunID:  "run_grandchild",
				Kind:   execution.ApprovalInterrupt,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "shell"},
				},
			},
			{
				ItemID: "item_b",
				RunID:  "run_b",
				Kind:   execution.ApprovalInterrupt,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "write"},
				},
			},
		},
		Suspensions: []SuspensionBinding{
			{InterruptItemID: "item_grandchild", ProcessID: "process_grandchild", SuspensionID: "suspension_grandchild"},
			{InterruptItemID: "item_b", ProcessID: "process_b", SuspensionID: "suspension_b"},
		},
		Continuations: []Continuation{
			{
				RunID:     "run_grandchild",
				ProcessID: "process_grandchild",
				Lineage: execution.RunLineage{
					SpawnedByItemID: "item_spawn_grandchild",
					ParentRunID:     "run_a",
					RootRunID:       "run_root",
				},
				RunCreatedAt: createdAt,
			},
			{
				RunID:     "run_a",
				ProcessID: "process_a",
				Lineage: execution.RunLineage{
					SpawnedByItemID: "item_spawn_a",
					ParentRunID:     "run_root",
					RootRunID:       "run_root",
				},
				RunCreatedAt: createdAt,
			},
			{
				RunID:     "run_b",
				ProcessID: "process_b",
				Lineage: execution.RunLineage{
					SpawnedByItemID: "item_spawn_b",
					ParentRunID:     "run_root",
					RootRunID:       "run_root",
				},
				RunCreatedAt: createdAt,
			},
			{
				RunID:        "run_root",
				ProcessID:    "process_root",
				RunCreatedAt: createdAt,
			},
		},
		CreatedAt: createdAt,
	}
}
