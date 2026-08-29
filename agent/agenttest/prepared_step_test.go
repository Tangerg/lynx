package agenttest_test

import (
	"errors"
	"testing"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/agenttest"
)

func TestPreparedStepRecorderConsumesFiniteFailures(t *testing.T) {
	injected := errors.New("durability unavailable")
	recorder := agenttest.NewPreparedStepRecorder(injected, nil)
	if err := recorder.AcknowledgePreparedStep(t.Context(), agent.ProcessSnapshot{}); !errors.Is(err, injected) {
		t.Fatalf("first acknowledgment error=%v", err)
	}
	if err := recorder.AcknowledgePreparedStep(t.Context(), agent.ProcessSnapshot{}); err != nil {
		t.Fatalf("second acknowledgment error=%v", err)
	}
	if err := recorder.AcknowledgePreparedStep(t.Context(), agent.ProcessSnapshot{}); err != nil {
		t.Fatalf("exhausted acknowledgment error=%v", err)
	}
	if len(recorder.Snapshots()) != 3 {
		t.Fatalf("snapshots=%d, want 3", len(recorder.Snapshots()))
	}
}
