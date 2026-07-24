package turn

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

// TestDoomLoopCounterTracksNoProgress checks the state machine that backs the
// brake: identical call+output grows the count; a changed output (progress) or a
// changed call resets it.
func TestDoomLoopCounterTracksNoProgress(t *testing.T) {
	st := &turnState{}
	args := `{"file_path":"x.go"}`

	st.recordToolOutcome("read", args, "content")
	if got := st.repeatedNoProgress("read", args); got != 1 {
		t.Fatalf("after 1 call: repeat = %d, want 1", got)
	}
	st.recordToolOutcome("read", args, "content")
	st.recordToolOutcome("read", args, "content")
	if got := st.repeatedNoProgress("read", args); got != 3 {
		t.Fatalf("after 3 identical calls: repeat = %d, want 3", got)
	}

	// A changed output = progress → reset (guards polling a live command).
	st.recordToolOutcome("read", args, "different")
	if got := st.repeatedNoProgress("read", args); got != 1 {
		t.Fatalf("changed output should reset: repeat = %d, want 1", got)
	}

	// A different call also resets, and the old key no longer matches.
	st.recordToolOutcome("read", args, "different")
	st.recordToolOutcome("grep", `{"pattern":"x"}`, "hit")
	if got := st.repeatedNoProgress("read", args); got != 0 {
		t.Fatalf("superseded key should report 0: repeat = %d", got)
	}
}

// TestDoomLoopBrakesRepeatedNoProgressCall drives the gate: an auto-pass call
// that has already run identically to no effect enough times is escalated to a
// human approval interrupt instead of silently passing.
func TestDoomLoopBrakesRepeatedNoProgressCall(t *testing.T) {
	// nil suspension → hitl.Interrupt raises a fresh SuspendedError we can decode.
	ctx := core.WithProcessView(t.Context(), suspendedProcessView{})
	// approval nil → Yolo → GatePass, so only the doom-loop brake can stop it.
	obs := &turnObserver{
		dispatcher: &memoryDispatcher{},
		st:         &turnState{handle: TurnHandle{SessionID: "s1"}},
	}
	args := `{"file_path":"x.go"}`

	// Below threshold: the call passes cleanly.
	for range doomLoopThreshold - 1 {
		obs.st.recordToolOutcome("read", args, "same")
	}
	if v := obs.ApproveToolCall(ctx, "c", "read", args, agentexec.ToolApprovalTarget{}); v.Interrupt != nil || v.Denied {
		t.Fatalf("below threshold should pass: %+v", v)
	}

	// At threshold: escalate to a doom-loop approval interrupt.
	obs.st.recordToolOutcome("read", args, "same")
	v := obs.ApproveToolCall(ctx, "c", "read", args, agentexec.ToolApprovalTarget{})
	if v.Interrupt == nil {
		t.Fatalf("at threshold should escalate, got %+v", v)
	}
	var suspended *interaction.SuspendedError
	if !errors.As(v.Interrupt, &suspended) {
		t.Fatalf("escalation should suspend for human input, got %v", v.Interrupt)
	}
	pending, err := suspension.DecodePrompt(suspended.Suspension.Prompt)
	if err != nil {
		t.Fatalf("decode doom-loop interrupt: %v", err)
	}
	if pending.Kind != runs.ApprovalInterruptKind || pending.Approval == nil {
		t.Fatalf("doom-loop interrupt = %+v, want an approval prompt", pending)
	}
	if pending.Approval.Rememberable {
		t.Fatal("doom-loop approval must not create a standing rule")
	}
	if !strings.Contains(strings.ToLower(pending.Approval.Reason), "loop") {
		t.Fatalf("approval reason should name the loop: %q", pending.Approval.Reason)
	}

	// The streak resets as the brake fires: an in-memory resume reuses this same
	// turnState, so without the reset the next identical call would re-trip the
	// brake and the model would never get room to continue after approval.
	if got := obs.st.repeatedNoProgress("read", args); got != 0 {
		t.Fatalf("escalation should reset the streak, got %d", got)
	}
}

// TestDoomLoopIgnoresProgressingPolls guards the false positive: a call repeated
// with a CHANGING result (e.g. polling a background command) never brakes.
func TestDoomLoopIgnoresProgressingPolls(t *testing.T) {
	ctx := core.WithProcessView(t.Context(), suspendedProcessView{})
	obs := &turnObserver{
		dispatcher: &memoryDispatcher{},
		st:         &turnState{handle: TurnHandle{SessionID: "s1"}},
	}
	args := `{"id":"bg-1"}`

	for _, out := range []string{"line 1", "line 2", "line 3", "line 4"} {
		obs.st.recordToolOutcome("shell_output", args, out)
	}
	if v := obs.ApproveToolCall(ctx, "c", "shell_output", args, agentexec.ToolApprovalTarget{}); v.Interrupt != nil || v.Denied {
		t.Fatalf("progressing polls must not brake: %+v", v)
	}
}
