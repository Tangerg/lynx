package turn

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// coalesceTextDeltas merges same-kind text deltas already queued on the channel
// so the per-token live stream collapses into fewer frames under load (T3.8).

func TestCoalesceTextDeltas_MergesConsecutive(t *testing.T) {
	ch := make(chan runs.EngineEvent, 8)
	ch <- runs.MessageDelta{Text: "b"}
	ch <- runs.MessageDelta{Text: "c"}
	var spill runs.EngineEvent
	got := coalesceTextDeltas(runs.MessageDelta{Text: "a"}, ch, &spill)
	if d, ok := got.(runs.MessageDelta); !ok || d.Text != "abc" {
		t.Fatalf("merged = %#v, want runs.MessageDelta{abc}", got)
	}
	if spill != nil {
		t.Fatalf("spill = %#v, want nil", spill)
	}
	if len(ch) != 0 {
		t.Fatalf("channel not drained: %d left", len(ch))
	}
}

func TestCoalesceTextDeltas_SpillsAtKindBoundary(t *testing.T) {
	ch := make(chan runs.EngineEvent, 8)
	ch <- runs.MessageDelta{Text: "b"}
	ch <- runs.TurnEnd{Reason: execution.OutcomeCompleted}
	ch <- runs.MessageDelta{Text: "c"} // past the boundary — must NOT be merged in
	var spill runs.EngineEvent
	got := coalesceTextDeltas(runs.MessageDelta{Text: "a"}, ch, &spill)
	if d, ok := got.(runs.MessageDelta); !ok || d.Text != "ab" {
		t.Fatalf("merged = %#v, want runs.MessageDelta{ab}", got)
	}
	if _, ok := spill.(runs.TurnEnd); !ok {
		t.Fatalf("spill = %#v, want runs.TurnEnd parked for the next yield", spill)
	}
	if len(ch) != 1 {
		t.Fatalf("channel has %d left, want 1 (the post-boundary delta)", len(ch))
	}
}

func TestCoalesceTextDeltas_PassesThroughNonDelta(t *testing.T) {
	ch := make(chan runs.EngineEvent, 4)
	ch <- runs.MessageDelta{Text: "x"}
	var spill runs.EngineEvent
	got := coalesceTextDeltas(runs.UsageReported{}, ch, &spill)
	if _, ok := got.(runs.UsageReported); !ok {
		t.Fatalf("got = %#v, want runs.UsageReported unchanged", got)
	}
	if spill != nil || len(ch) != 1 {
		t.Fatalf("a non-delta head must not touch ch/spill: spill=%#v len=%d", spill, len(ch))
	}
}

func TestCoalesceTextDeltas_ReasoningMergesByKind(t *testing.T) {
	ch := make(chan runs.EngineEvent, 8)
	ch <- runs.ReasoningDelta{Text: "2"}
	ch <- runs.MessageDelta{Text: "x"} // different kind → spilled, not merged
	var spill runs.EngineEvent
	got := coalesceTextDeltas(runs.ReasoningDelta{Text: "1"}, ch, &spill)
	if r, ok := got.(runs.ReasoningDelta); !ok || r.Text != "12" {
		t.Fatalf("merged = %#v, want runs.ReasoningDelta{12}", got)
	}
	if _, ok := spill.(runs.MessageDelta); !ok {
		t.Fatalf("spill = %#v, want runs.MessageDelta", spill)
	}
}
