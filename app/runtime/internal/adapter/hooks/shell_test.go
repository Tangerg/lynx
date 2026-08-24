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

func TestShellBoundsCommandOutputAndRejectsMalformedDecision(t *testing.T) {
	t.Run("oversized stdout", func(t *testing.T) {
		got := Shell{}.RunHookCommand(t.Context(), apphooks.CommandRequest{
			Command: `printf '{"injectContext":"'; head -c 70000 /dev/zero | tr '\000' x; printf '"}'`,
			Timeout: time.Second,
		})
		if got.Err == nil {
			t.Fatal("oversized hook stdout was accepted")
		}
		if got.Decision.InjectContext != "" {
			t.Fatalf("oversized stdout reached the decision: %d bytes", len(got.Decision.InjectContext))
		}
	})

	t.Run("bounded stderr", func(t *testing.T) {
		got := Shell{}.RunHookCommand(t.Context(), apphooks.CommandRequest{
			Command: `head -c 70000 /dev/zero | tr '\000' x >&2`,
			Timeout: time.Second,
		})
		if len(got.Stderr) > 64<<10 {
			t.Fatalf("hook stderr retained %d bytes, want at most 64 KiB", len(got.Stderr))
		}
	})

	for _, output := range []string{
		`not-json`,
		`null`,
		`{"decision":"unknown"}`,
		`{"unknown":true}`,
		`{"decision":"deny"} {"decision":"allow"}`,
	} {
		t.Run(output, func(t *testing.T) {
			got := Shell{}.RunHookCommand(t.Context(), apphooks.CommandRequest{
				Command: `printf '%s' ` + shellSingleQuote(output),
				Timeout: time.Second,
			})
			if got.Err == nil {
				t.Fatalf("malformed hook decision %q was silently accepted", output)
			}
		})
	}
}

func TestShellRejectsUnboundedCommandInput(t *testing.T) {
	tests := []struct {
		name  string
		input domainhooks.Input
	}{
		{
			name: "prompt",
			input: domainhooks.Input{
				Event:  domainhooks.UserPromptSubmit,
				Prompt: strings.Repeat("p", (256<<10)+1),
			},
		},
		{
			name: "tool arguments",
			input: domainhooks.Input{
				Event: domainhooks.PreToolUse,
				Tool: &domainhooks.ToolInput{
					Name: "shell", Arguments: strings.Repeat("a", (256<<10)+1),
				},
			},
		},
		{
			name: "tool result",
			input: domainhooks.Input{
				Event: domainhooks.PostToolUse,
				Tool: &domainhooks.ToolInput{
					Name: "shell", Result: strings.Repeat("r", (128<<10)+1),
				},
			},
		},
		{
			name: "reason",
			input: domainhooks.Input{
				Event: domainhooks.Stop, Reason: strings.Repeat("e", (8<<10)+1),
			},
		},
		{
			name: "aggregate stdin",
			input: domainhooks.Input{
				Event: domainhooks.SessionStart, SessionID: strings.Repeat("s", (512<<10)+1),
			},
		},
		{
			name: "encoded stdin",
			input: domainhooks.Input{
				Event: domainhooks.SessionStart, SessionID: strings.Repeat("\x00", 100<<10),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Shell{}.RunHookCommand(t.Context(), apphooks.CommandRequest{
				Command: ":", Input: test.input, Timeout: time.Second,
			})
			if got.Err == nil || got.ExitCode != -1 {
				t.Fatalf("oversized %s result = %+v, want pre-spawn rejection", test.name, got)
			}
		})
	}
}

func TestHookInputWirePreservesBoundedProjectionMarkers(t *testing.T) {
	prompt := hookInputWireFrom(domainhooks.Input{
		Event: domainhooks.UserPromptSubmit, Prompt: "prefix", PromptTruncated: true,
	})
	if !prompt.PromptTruncated {
		t.Fatal("prompt truncation marker was dropped at the process boundary")
	}
	tool := hookInputWireFrom(domainhooks.Input{
		Event: domainhooks.PostToolUse,
		Tool:  &domainhooks.ToolInput{Name: "shell", Result: "prefix", ResultTruncated: true},
	})
	if tool.Tool == nil || !tool.Tool.ResultTruncated {
		t.Fatal("tool result truncation marker was dropped at the process boundary")
	}
	subagent := hookInputWireFrom(domainhooks.Input{
		Event: domainhooks.SubagentStop,
		Subagent: &domainhooks.SubagentInput{
			RunID: "run-child", ParentRunID: "run-root",
			Prompt: "prompt", PromptTruncated: true,
			Result: "result", ResultTruncated: true,
		},
	})
	if subagent.Subagent == nil ||
		!subagent.Subagent.PromptTruncated ||
		!subagent.Subagent.ResultTruncated {
		t.Fatal("sub-agent truncation markers were dropped at the process boundary")
	}
}

func TestShellOversizedExitTwoRemainsAnExplicitDeny(t *testing.T) {
	runner := apphooks.NewRunner(Shell{}, nil)
	decision := runner.Run(t.Context(), []domainhooks.Hook{{
		Event: domainhooks.PreToolUse, Source: "/tmp/hooks.json",
		Command: `printf '{"injectContext":"'; head -c 70000 /dev/zero | tr '\000' x; printf '"}'; printf 'blocked' >&2; exit 2`,
	}}, domainhooks.Input{
		Event: domainhooks.PreToolUse,
		Tool:  &domainhooks.ToolInput{Name: "shell"},
	})
	if !decision.Block || decision.Reason != "blocked" {
		t.Fatalf("oversized exit-two decision = %+v, want explicit bounded deny", decision)
	}
}

func TestShellMalformedDecisionIsObservableAndNonBlocking(t *testing.T) {
	var observed error
	runner := apphooks.NewRunner(Shell{}, func(_ context.Context, _ string, err error) {
		observed = err
	})
	decision := runner.Run(t.Context(), []domainhooks.Hook{{
		Event: domainhooks.PreToolUse, Source: "/tmp/hooks.json",
		Command: `printf 'not-json'`,
	}}, domainhooks.Input{
		Event: domainhooks.PreToolUse,
		Tool:  &domainhooks.ToolInput{Name: "shell"},
	})
	if decision.Block || observed == nil {
		t.Fatalf("malformed decision = %+v observed=%v, want observable non-blocking failure", decision, observed)
	}
}

func TestHookOutputBufferDrainsAfterItsBoundedPrefix(t *testing.T) {
	buffer := newHookOutputBuffer(4)
	if written, err := buffer.Write([]byte("abcdef")); err != nil || written != 6 {
		t.Fatalf("Write = (%d, %v), want (6, nil)", written, err)
	}
	if got := buffer.String(); got != "abcd" || !buffer.overflow {
		t.Fatalf("buffer = %q overflow=%v, want bounded overflowing prefix", got, buffer.overflow)
	}
}

func TestShellTimeoutKillsDescendantProcessGroup(t *testing.T) {
	started := time.Now()
	got := Shell{}.RunHookCommand(t.Context(), apphooks.CommandRequest{
		Command: `sleep 5 & wait`,
		Timeout: 40 * time.Millisecond,
	})
	if !got.TimedOut {
		t.Fatalf("TimedOut = false, err=%v", got.Err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timed-out hook retained a descendant process for %s", elapsed)
	}
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
