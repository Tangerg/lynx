package hooks

import (
	"errors"
	"testing"
)

func TestHookValidate(t *testing.T) {
	tests := []struct {
		name string
		hook Hook
		ok   bool
	}{
		{name: "command", hook: Hook{Event: PreToolUse, Command: "check"}, ok: true},
		{name: "inject", hook: Hook{Event: SessionStart, Inject: "context"}, ok: true},
		{name: "unknown event", hook: Hook{Event: "PreTool", Command: "check"}},
		{name: "missing action", hook: Hook{Event: Stop}},
		{name: "ambiguous action", hook: Hook{Event: Stop, Command: "notify", Inject: "context"}},
		{name: "blank command", hook: Hook{Event: Stop, Command: "  "}},
		{name: "negative timeout", hook: Hook{Event: Stop, Command: "notify", TimeoutMs: -1}},
		{name: "timeout on inject", hook: Hook{Event: SessionStart, Inject: "context", TimeoutMs: 100}},
		{name: "matcher on non-tool event", hook: Hook{Event: Stop, Command: "notify", Matcher: "shell"}},
		{name: "malformed matcher", hook: Hook{Event: PreToolUse, Command: "check", Matcher: "["}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.hook.Validate()
			if test.ok && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if !test.ok && !errors.Is(err, ErrInvalidHook) {
				t.Fatalf("Validate error = %v, want ErrInvalidHook", err)
			}
		})
	}
}
