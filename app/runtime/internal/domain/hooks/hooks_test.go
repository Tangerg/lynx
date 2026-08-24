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

func TestCommandProjectionBoundsDynamicMaterialWithoutMutatingInput(t *testing.T) {
	input := Input{
		Event:  PostToolUse,
		Prompt: strings.Repeat("p", MaxPromptBytes+1),
		Reason: strings.Repeat("e", MaxReasonBytes+1),
		Tool: &ToolInput{
			Name: "shell", Arguments: `{"command":"go test ./..."}`,
			Result: strings.Repeat("r", MaxResultBytes+1),
		},
		Subagent: &SubagentInput{
			RunID: "run-child", ParentRunID: "run-root",
			Description: strings.Repeat("d", MaxReasonBytes+1),
			Prompt:      strings.Repeat("q", MaxPromptBytes+1),
			Result:      strings.Repeat("s", MaxResultBytes+1),
			Error:       strings.Repeat("x", MaxReasonBytes+1),
		},
	}

	projected, err := input.CommandProjection()
	if err != nil {
		t.Fatalf("CommandProjection: %v", err)
	}
	if len(projected.Prompt) != MaxPromptBytes || !projected.PromptTruncated {
		t.Fatalf("prompt = %d bytes truncated=%v", len(projected.Prompt), projected.PromptTruncated)
	}
	if len(projected.Reason) != MaxReasonBytes {
		t.Fatalf("reason = %d bytes, want %d", len(projected.Reason), MaxReasonBytes)
	}
	if projected.Tool == input.Tool ||
		len(projected.Tool.Result) != MaxResultBytes ||
		!projected.Tool.ResultTruncated ||
		projected.Tool.Arguments != input.Tool.Arguments {
		t.Fatalf("projected tool = %+v", projected.Tool)
	}
	if projected.Subagent == input.Subagent ||
		len(projected.Subagent.Description) != MaxReasonBytes ||
		len(projected.Subagent.Prompt) != MaxPromptBytes ||
		!projected.Subagent.PromptTruncated ||
		len(projected.Subagent.Result) != MaxResultBytes ||
		!projected.Subagent.ResultTruncated ||
		len(projected.Subagent.Error) != MaxReasonBytes {
		t.Fatalf("projected subagent = %+v", projected.Subagent)
	}
	if len(input.Prompt) != MaxPromptBytes+1 ||
		len(input.Tool.Result) != MaxResultBytes+1 ||
		len(input.Subagent.Prompt) != MaxPromptBytes+1 {
		t.Fatal("CommandProjection mutated its input")
	}
}

func TestCommandProjectionRejectsLossyToolArguments(t *testing.T) {
	_, err := (Input{
		Event: PreToolUse,
		Tool: &ToolInput{
			Name: "shell", Arguments: strings.Repeat("a", MaxArgumentsBytes+1),
		},
	}).CommandProjection()
	if !errors.Is(err, ErrCommandInputTooLarge) {
		t.Fatalf("CommandProjection error = %v, want ErrCommandInputTooLarge", err)
	}

	_, err = (Input{
		Event: PreToolUse,
		Tool:  &ToolInput{Name: "shell", Arguments: "{\xff}"},
	}).CommandProjection()
	if !errors.Is(err, ErrInvalidCommandInput) {
		t.Fatalf("invalid UTF-8 arguments error = %v, want ErrInvalidCommandInput", err)
	}
}

func TestValidateCommandMaterialAcceptsExactBoundaries(t *testing.T) {
	tests := []Input{
		{Event: UserPromptSubmit, Prompt: strings.Repeat("p", MaxPromptBytes)},
		{Event: Stop, Reason: strings.Repeat("e", MaxReasonBytes)},
		{
			Event: PreToolUse,
			Tool:  &ToolInput{Name: "shell", Arguments: strings.Repeat("a", MaxArgumentsBytes)},
		},
		{
			Event: PostToolUse,
			Tool:  &ToolInput{Name: "shell", Result: strings.Repeat("r", MaxResultBytes)},
		},
	}
	for index, input := range tests {
		if err := input.ValidateCommandMaterial(); err != nil {
			t.Fatalf("exact boundary input[%d]: %v", index, err)
		}
	}
}
