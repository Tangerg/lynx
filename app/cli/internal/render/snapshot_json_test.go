package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestRunJSONPreservesNegotiatedProtocolProfile(t *testing.T) {
	t.Parallel()

	run := agent.Run{
		ID: "run_1", SessionID: "session_1", Status: agent.RunStatusRunning, ActiveSegmentID: "segment_1",
		Contract: &agent.RunContract{
			RequiredFeatures: []agent.RunFeature{agent.RunFeatureSubagents},
			InteractionKinds: []agent.InteractionKind{agent.InteractionApproval, agent.InteractionQuestion},
		},
	}
	var output bytes.Buffer
	if err := WriteRunJSON(&output, run); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"protocolProfile"`, `"requiredFeatures":["subagents"]`, `"interruptTypes":["approval","question"]`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("run JSON omitted %s: %s", want, output.String())
		}
	}
}
