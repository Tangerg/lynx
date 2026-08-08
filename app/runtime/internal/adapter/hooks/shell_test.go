package hooks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	apphooks "github.com/Tangerg/lynx/app/runtime/internal/application/hooks"
	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestShell_CommandReceivesStdin(t *testing.T) {
	got := Shell{}.RunHookCommand(context.Background(), apphooks.CommandRequest{
		Command: `grep -q '"event":"UserPromptSubmit"' && echo '{"injectContext":"saw-event"}'`,
		Input:   domainhooks.Input{Event: domainhooks.UserPromptSubmit},
		Timeout: time.Second,
	})
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if strings.TrimSpace(got.Decision.InjectContext) != "saw-event" {
		t.Fatalf("decision = %+v", got.Decision)
	}
}

func TestSubagentHookWireUsesApplicationRunIdentity(t *testing.T) {
	encoded, err := json.Marshal(hookInputWireFrom(domainhooks.Input{
		Event: domainhooks.SubagentStart,
		Subagent: &domainhooks.SubagentInput{
			RunID: "run-child", ParentRunID: "run-root", Description: "inspect auth",
		},
	}))
	if err != nil {
		t.Fatalf("marshal hook input: %v", err)
	}
	wire := string(encoded)
	for _, required := range []string{`"runId":"run-child"`, `"parentRunId":"run-root"`} {
		if !strings.Contains(wire, required) {
			t.Fatalf("hook wire %s is missing %s", wire, required)
		}
	}
	if strings.Contains(wire, "processId") || strings.Contains(wire, "parentProcessId") {
		t.Fatalf("hook wire leaks Framework process identity: %s", wire)
	}
}

func TestShell_Timeout(t *testing.T) {
	got := Shell{}.RunHookCommand(context.Background(), apphooks.CommandRequest{
		Command: `sleep 5`,
		Timeout: 40 * time.Millisecond,
	})
	if !got.TimedOut {
		t.Fatalf("TimedOut = false, err=%v", got.Err)
	}
}
