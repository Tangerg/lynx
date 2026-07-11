package turn

import "testing"

// coalesceTextDeltas merges same-kind text deltas already queued on the channel
// so the per-token live stream collapses into fewer frames under load (T3.8).

func TestCoalesceTextDeltas_MergesConsecutive(t *testing.T) {
	ch := make(chan Event, 8)
	ch <- MessageDelta{Text: "b"}
	ch <- MessageDelta{Text: "c"}
	var spill Event
	got := coalesceTextDeltas(MessageDelta{Text: "a"}, ch, &spill)
	if d, ok := got.(MessageDelta); !ok || d.Text != "abc" {
		t.Fatalf("merged = %#v, want MessageDelta{abc}", got)
	}
	if spill != nil {
		t.Fatalf("spill = %#v, want nil", spill)
	}
	if len(ch) != 0 {
		t.Fatalf("channel not drained: %d left", len(ch))
	}
}

func TestCoalesceTextDeltas_SpillsAtKindBoundary(t *testing.T) {
	ch := make(chan Event, 8)
	ch <- MessageDelta{Text: "b"}
	ch <- TurnEnd{Reason: TurnEndCompleted}
	ch <- MessageDelta{Text: "c"} // past the boundary — must NOT be merged in
	var spill Event
	got := coalesceTextDeltas(MessageDelta{Text: "a"}, ch, &spill)
	if d, ok := got.(MessageDelta); !ok || d.Text != "ab" {
		t.Fatalf("merged = %#v, want MessageDelta{ab}", got)
	}
	if _, ok := spill.(TurnEnd); !ok {
		t.Fatalf("spill = %#v, want TurnEnd parked for the next yield", spill)
	}
	if len(ch) != 1 {
		t.Fatalf("channel has %d left, want 1 (the post-boundary delta)", len(ch))
	}
}

func TestCoalesceTextDeltas_PassesThroughNonDelta(t *testing.T) {
	ch := make(chan Event, 4)
	ch <- MessageDelta{Text: "x"}
	var spill Event
	got := coalesceTextDeltas(TurnStart{}, ch, &spill)
	if _, ok := got.(TurnStart); !ok {
		t.Fatalf("got = %#v, want TurnStart unchanged", got)
	}
	if spill != nil || len(ch) != 1 {
		t.Fatalf("a non-delta head must not touch ch/spill: spill=%#v len=%d", spill, len(ch))
	}
}

func TestCoalesceTextDeltas_ReasoningMergesByKind(t *testing.T) {
	ch := make(chan Event, 8)
	ch <- ReasoningDelta{Text: "2"}
	ch <- MessageDelta{Text: "x"} // different kind → spilled, not merged
	var spill Event
	got := coalesceTextDeltas(ReasoningDelta{Text: "1"}, ch, &spill)
	if r, ok := got.(ReasoningDelta); !ok || r.Text != "12" {
		t.Fatalf("merged = %#v, want ReasoningDelta{12}", got)
	}
	if _, ok := spill.(MessageDelta); !ok {
		t.Fatalf("spill = %#v, want MessageDelta", spill)
	}
}
