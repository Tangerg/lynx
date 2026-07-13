package server

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// resumeOKTurns is a turn dispatcher whose Resume succeeds and whose Cancel is a
// no-op — enough to carry ResumeRun past the interrupt consume + turn resume so
// the failing continuation Start is what's under test.
type resumeOKTurns struct{ turn.Dispatcher }

func (resumeOKTurns) Resume(context.Context, turn.TurnHandle, interrupts.Resolution, []string) error {
	return nil
}
func (resumeOKTurns) Cancel(context.Context, turn.TurnHandle) error { return nil }
func (resumeOKTurns) ProcessID(_ context.Context, handle turn.TurnHandle) (string, error) {
	return handle.TurnID, nil
}

// TestResumeRun_KeepsInterruptOpenWhenStartFails proves ownership ordering: the
// continuation must durably open before its parked decision is delivered. A
// pre-opening Start failure therefore leaves the interrupt untouched and retryable
// without a compensation write.
func TestResumeRun_KeepsInterruptOpenWhenStartFails(t *testing.T) {
	s, rt := rollbackHarness(t)
	rt.turns = resumeOKTurns{}
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")

	if err := rt.interrupts.Put(ctx, interrupts.Pending{
		RunID:      "run_1",
		SessionID:  sess.ID,
		TurnID:     "turn_parked",
		ProcessID:  "turn_parked",
		Provider:   "openai",
		Model:      "gpt",
		Interrupts: []byte(`[]`),
	}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	// Close the run coordinator so continuation admission fails before opening.
	s.coordinator.Close()

	if _, _, err := s.ResumeRun(ctx, protocol.ResumeRunRequest{RunID: "run_1"}); err == nil {
		t.Fatal("ResumeRun must surface the failed continuation Start")
	}

	// No compensation is needed: the opening transaction never consumed it.
	if _, found, err := rt.interrupts.Get(ctx, "run_1"); err != nil || !found {
		t.Fatalf("interrupt changed after rejected resume Start (found=%v err=%v)", found, err)
	}
}
