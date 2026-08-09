package interaction

import (
	"encoding/json"
	"math"
	"testing"
)

func TestToolBatchPauseCountDoesNotWrap(t *testing.T) {
	request, err := NewToolInputRequest(
		json.RawMessage(`"provide another value"`),
		json.RawMessage(`{"type":"string"}`),
		json.RawMessage(`{"continuation":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	dispatch := toolBatchDispatch{pauseCount: math.MaxUint32}
	if _, err := dispatch.pause(0, request); err == nil {
		t.Fatal("exhausted Tool input pause count wrapped instead of failing")
	}
	if dispatch.pauseCount != math.MaxUint32 {
		t.Fatalf("pause count changed to %d", dispatch.pauseCount)
	}
}
