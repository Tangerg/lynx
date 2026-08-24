package hooks

import (
	"errors"
	"strings"
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
		{
			name: "exact resource boundary",
			hook: Hook{
				Event: PreToolUse, Matcher: strings.Repeat("x", MaxMatcherBytes),
				Command: strings.Repeat("x", MaxActionBytes), TimeoutMillis: MaxTimeoutMillis,
			},
			ok: true,
		},
		{name: "unknown event", hook: Hook{Event: "PreTool", Command: "check"}},
		{name: "missing action", hook: Hook{Event: Stop}},
		{name: "ambiguous action", hook: Hook{Event: Stop, Command: "notify", Inject: "context"}},
		{name: "blank command", hook: Hook{Event: Stop, Command: "  "}},
		{name: "negative timeout", hook: Hook{Event: Stop, Command: "notify", TimeoutMillis: -1}},
		{name: "timeout on inject", hook: Hook{Event: SessionStart, Inject: "context", TimeoutMillis: 100}},
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

func TestHookConfigurationCollectionEnvelope(t *testing.T) {
	if err := ValidateConfigurationFileSize(MaxConfigurationFileBytes); err != nil {
		t.Fatalf("exact file boundary: %v", err)
	}
	if err := ValidateConfigurationFileSize(MaxConfigurationFileBytes + 1); !errors.Is(err, ErrConfigurationTooLarge) {
		t.Fatalf("oversized file error = %v, want ErrConfigurationTooLarge", err)
	}
	if err := ValidateHooksPerFile(MaxHooksPerFile); err != nil {
		t.Fatalf("exact file hook count: %v", err)
	}
	if err := ValidateHooksPerFile(MaxHooksPerFile + 1); !errors.Is(err, ErrConfigurationTooLarge) {
		t.Fatalf("overfull file error = %v, want ErrConfigurationTooLarge", err)
	}
	if err := ValidateHookCascade(MaxHooksPerCascade); err != nil {
		t.Fatalf("exact cascade hook count: %v", err)
	}
	if err := ValidateHookCascade(MaxHooksPerCascade + 1); !errors.Is(err, ErrConfigurationTooLarge) {
		t.Fatalf("overfull cascade error = %v, want ErrConfigurationTooLarge", err)
	}
}

func TestHookValidateRejectsUnboundedConfiguration(t *testing.T) {
	tests := []struct {
		name string
		hook Hook
	}{
		{
			name: "matcher",
			hook: Hook{Event: PreToolUse, Matcher: strings.Repeat("x", 257), Command: "check"},
		},
		{
			name: "command",
			hook: Hook{Event: Stop, Command: strings.Repeat("x", (8<<10)+1)},
		},
		{
			name: "inject",
			hook: Hook{Event: SessionStart, Inject: strings.Repeat("x", (8<<10)+1)},
		},
		{
			name: "timeout",
			hook: Hook{Event: Stop, Command: "check", TimeoutMillis: (5 * 60 * 1_000) + 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.hook.Validate(); !errors.Is(err, ErrInvalidHook) {
				t.Fatalf("Validate error = %v, want ErrInvalidHook", err)
			}
		})
	}
}
