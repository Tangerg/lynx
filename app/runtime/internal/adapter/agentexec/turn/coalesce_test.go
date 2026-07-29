package turn

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func TestCoalesceTextDeltas_MergesConsecutive(t *testing.T) {
	ch := make(chan runs.ExecutorEvent, 8)
	ch <- rootExecutorEvent(runs.MessageDelta{Text: "b"})
	ch <- rootExecutorEvent(runs.MessageDelta{Text: "c"})
	var spill *runs.ExecutorEvent
	got := coalesceTextDeltas(rootExecutorEvent(runs.MessageDelta{Text: "a"}), ch, &spill)
	if d, ok := got.Payload.(runs.MessageDelta); !ok || d.Text != "abc" {
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
	ch := make(chan runs.ExecutorEvent, 8)
	ch <- rootExecutorEvent(runs.MessageDelta{Text: "b"})
	ch <- rootExecutorEvent(runs.TurnEnd{Reason: execution.OutcomeCompleted})
	ch <- rootExecutorEvent(runs.MessageDelta{Text: "c"}) // past the boundary — must NOT be merged in
	var spill *runs.ExecutorEvent
	got := coalesceTextDeltas(rootExecutorEvent(runs.MessageDelta{Text: "a"}), ch, &spill)
	if d, ok := got.Payload.(runs.MessageDelta); !ok || d.Text != "ab" {
		t.Fatalf("merged = %#v, want runs.MessageDelta{ab}", got)
	}
	if spill == nil {
		t.Fatal("spill is nil, want runs.TurnEnd")
	}
	if _, ok := spill.Payload.(runs.TurnEnd); !ok {
		t.Fatalf("spill = %#v, want runs.TurnEnd parked for the next yield", spill)
	}
	if len(ch) != 1 {
		t.Fatalf("channel has %d left, want 1 (the post-boundary delta)", len(ch))
	}
}

func TestCoalesceTextDeltas_PassesThroughNonDelta(t *testing.T) {
	ch := make(chan runs.ExecutorEvent, 4)
	ch <- rootExecutorEvent(runs.MessageDelta{Text: "x"})
	var spill *runs.ExecutorEvent
	got := coalesceTextDeltas(rootExecutorEvent(runs.UsageReported{}), ch, &spill)
	if _, ok := got.Payload.(runs.UsageReported); !ok {
		t.Fatalf("got = %#v, want runs.UsageReported unchanged", got)
	}
	if spill != nil || len(ch) != 1 {
		t.Fatalf("a non-delta head must not touch ch/spill: spill=%#v len=%d", spill, len(ch))
	}
}

func TestCoalesceTextDeltas_ReasoningMergesByKind(t *testing.T) {
	ch := make(chan runs.ExecutorEvent, 8)
	ch <- rootExecutorEvent(runs.ReasoningDelta{Text: "2"})
	ch <- rootExecutorEvent(runs.MessageDelta{Text: "x"}) // different kind → spilled, not merged
	var spill *runs.ExecutorEvent
	got := coalesceTextDeltas(rootExecutorEvent(runs.ReasoningDelta{Text: "1"}), ch, &spill)
	if r, ok := got.Payload.(runs.ReasoningDelta); !ok || r.Text != "12" {
		t.Fatalf("merged = %#v, want runs.ReasoningDelta{12}", got)
	}
	if spill == nil {
		t.Fatal("spill is nil, want runs.MessageDelta")
	}
	if _, ok := spill.Payload.(runs.MessageDelta); !ok {
		t.Fatalf("spill = %#v, want runs.MessageDelta", spill)
	}
}

func TestCoalesceTextDeltas_SpillsAtProcessBoundary(t *testing.T) {
	root := runs.ExecutorSource{ProcessID: "process_root"}
	child := runs.ExecutorSource{
		ProcessID:   "process_child",
		ParentID:    root.ProcessID,
		SpawnCallID: "call_delegate",
	}
	ch := make(chan runs.ExecutorEvent, 2)
	ch <- runs.ExecutorEvent{Source: child, Payload: runs.MessageDelta{Text: "child"}}
	ch <- runs.ExecutorEvent{Source: root, Payload: runs.MessageDelta{Text: "after"}}

	var spill *runs.ExecutorEvent
	head := runs.ExecutorEvent{Source: root, Payload: runs.MessageDelta{Text: "before"}}
	got := coalesceTextDeltas(head, ch, &spill)
	if delta, ok := got.Payload.(runs.MessageDelta); !ok || delta.Text != "before" {
		t.Fatalf("head = %#v, want unmerged root delta", got)
	}
	if got.Source != root {
		t.Fatalf("head source = %+v, want %+v", got.Source, root)
	}
	if spill == nil || spill.Source != child {
		t.Fatalf("spill = %#v, want child event", spill)
	}
	if delta, ok := spill.Payload.(runs.MessageDelta); !ok || delta.Text != "child" {
		t.Fatalf("spill payload = %#v, want child delta", spill.Payload)
	}
	if len(ch) != 1 {
		t.Fatalf("channel has %d events left, want post-boundary root delta", len(ch))
	}
}

func TestCoalesceTextDeltas_DrainsBufferedClosedChannel(t *testing.T) {
	ch := make(chan runs.ExecutorEvent, 2)
	ch <- rootExecutorEvent(runs.MessageDelta{Text: "b"})
	ch <- rootExecutorEvent(runs.MessageDelta{Text: "c"})
	close(ch)

	var spill *runs.ExecutorEvent
	got := coalesceTextDeltas(rootExecutorEvent(runs.MessageDelta{Text: "a"}), ch, &spill)
	if delta, ok := got.Payload.(runs.MessageDelta); !ok || delta.Text != "abc" {
		t.Fatalf("merged = %#v, want runs.MessageDelta{abc}", got)
	}
	if spill != nil {
		t.Fatalf("spill = %#v, want nil", spill)
	}
}

func BenchmarkCoalesceTextDeltas(b *testing.B) {
	const buffered = 32
	ch := make(chan runs.ExecutorEvent, buffered)
	delta := rootExecutorEvent(runs.MessageDelta{Text: "x"})
	for b.Loop() {
		for range buffered {
			ch <- delta
		}
		head := <-ch
		var spill *runs.ExecutorEvent
		got := coalesceTextDeltas(head, ch, &spill)
		if len(got.Payload.(runs.MessageDelta).Text) != buffered {
			b.Fatal("coalesced text has the wrong length")
		}
	}
}

func rootExecutorEvent(payload runs.EngineEvent) runs.ExecutorEvent {
	return runs.ExecutorEvent{
		Source:  runs.ExecutorSource{ProcessID: "process_root"},
		Payload: payload,
	}
}
