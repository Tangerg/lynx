package transcript_test

import (
	"math"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func TestSequenceOrderIsAClosedVocabulary(t *testing.T) {
	for _, order := range []transcript.SequenceOrder{transcript.OldestFirst, transcript.NewestFirst} {
		if err := order.Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", order, err)
		}
		if order.String() != string(order) {
			t.Fatalf("String(%q) = %q", order, order.String())
		}
	}
	for _, order := range []transcript.SequenceOrder{"", "ascending", "descending"} {
		if err := order.Validate(); err == nil {
			t.Fatalf("Validate(%q) succeeded", order)
		}
	}
}

func TestCloneContentOwnsImageBytes(t *testing.T) {
	original := []transcript.ContentBlock{{
		Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte{1, 2, 3},
	}}
	cloned := transcript.CloneContent(original)

	cloned[0].Bytes[0] = 9
	if original[0].Bytes[0] != 1 {
		t.Fatalf("CloneContent shares image storage: original bytes = %v", original[0].Bytes)
	}
	original[0].Bytes[1] = 8
	if cloned[0].Bytes[1] != 2 {
		t.Fatalf("CloneContent is not ownership-isolated: cloned bytes = %v", cloned[0].Bytes)
	}
}

func TestItemValidateOwnsPayloadInvariants(t *testing.T) {
	identity := transcript.ItemIdentity{
		SessionID: "session-1", RunID: "run-1", ItemID: "item-1",
		OccurredAt: time.Unix(1, 0).UTC(),
	}
	tests := []struct {
		name     string
		snapshot transcript.ItemSnapshot
		wantErr  bool
	}{
		{
			name: "tool data on user message",
			snapshot: transcript.ItemSnapshot{
				Identity: identity, Kind: transcript.UserMessage, Status: transcript.ItemCompleted,
				Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
				Tool:    &transcript.ToolInvocation{Name: "shell"},
			},
			wantErr: true,
		},
		{
			name: "missing occurrence time",
			snapshot: transcript.ItemSnapshot{
				Identity: transcript.ItemIdentity{SessionID: "session-1", RunID: "run-1", ItemID: "item-1"},
				Kind:     transcript.UserMessage, Status: transcript.ItemCompleted,
				Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
			},
			wantErr: true,
		},
		{
			name: "valid message",
			snapshot: transcript.ItemSnapshot{
				Identity: identity, Kind: transcript.UserMessage, Status: transcript.ItemCompleted,
				Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := transcript.RestoreItem(test.snapshot)
			if (err != nil) != test.wantErr {
				t.Fatalf("RestoreItem() error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}

func TestToolCallTimingLifecycle(t *testing.T) {
	startedAt := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	identity := transcript.ItemIdentity{
		SessionID: "session-1", RunID: "run-1", ItemID: "item-1", OccurredAt: startedAt,
	}
	invocation := transcript.ToolInvocation{Name: "shell"}
	running, err := transcript.NewToolCall(identity, invocation, tool.SafetyClass(""))
	if err != nil {
		t.Fatalf("NewToolCall(): %v", err)
	}
	if running.Status() != transcript.ItemRunning || !running.FinishedAt().IsZero() {
		t.Fatalf("running timing = (%v, %v)", running.Status(), running.FinishedAt())
	}
	completed, err := running.CompleteToolCall(invocation, finishedAt)
	if err != nil {
		t.Fatalf("CompleteToolCall(): %v", err)
	}
	if completed.Status() != transcript.ItemCompleted || !completed.FinishedAt().Equal(finishedAt) {
		t.Fatalf("completed timing = (%v, %v)", completed.Status(), completed.FinishedAt())
	}
	if _, err := completed.CompleteToolCall(invocation, finishedAt.Add(time.Second)); err == nil {
		t.Fatal("second CompleteToolCall() succeeded")
	}
	if _, err := running.CompleteToolCall(invocation, startedAt.Add(-time.Millisecond)); err == nil {
		t.Fatal("CompleteToolCall() accepted a finish before start")
	}
	abandoned, err := running.AbandonToolCall(nil, finishedAt)
	if err != nil {
		t.Fatalf("AbandonToolCall(): %v", err)
	}
	if abandoned.Status() != transcript.ItemIncomplete {
		t.Fatalf("abandoned status = %v", abandoned.Status())
	}
}

func TestUsageAndToolFailureValidate(t *testing.T) {
	negativeCost := -0.1
	nanCost := math.NaN()
	infiniteCost := math.Inf(1)
	tests := []struct {
		name    string
		usage   *accounting.Usage
		failure *tool.Failure
		wantErr bool
	}{
		{name: "valid"},
		{name: "negative token", usage: &accounting.Usage{Total: accounting.Totals{InputTokens: -1}}, wantErr: true},
		{name: "negative cost", usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &negativeCost}}, wantErr: true},
		{name: "nan cost", usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &nanCost}}, wantErr: true},
		{name: "infinite cost", usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &infiniteCost}}, wantErr: true},
		{name: "unknown failure kind", failure: &tool.Failure{Kind: tool.FailureKind(99)}, wantErr: true},
		{name: "retry delay on permanent failure", failure: &tool.Failure{Kind: tool.FailureDenied, RetryAfter: time.Second}, wantErr: true},
		{name: "valid execution failure", failure: &tool.Failure{Kind: tool.FailureExecution}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			if test.usage != nil {
				err = test.usage.Validate()
			}
			if err == nil && test.failure != nil {
				err = test.failure.Validate()
			}
			if (err != nil) != test.wantErr {
				t.Fatalf("validation error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}

func TestQuestionValidateRejectsUnanswerableShapes(t *testing.T) {
	choice := func(options ...string) []transcript.QuestionOption {
		result := make([]transcript.QuestionOption, len(options))
		for index, option := range options {
			result[index] = transcript.QuestionOption{Label: option}
		}
		return result
	}
	tests := []struct {
		name    string
		value   transcript.Question
		wantErr bool
	}{
		{name: "text", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Explain", Kind: transcript.QuestionText}}}},
		{name: "custom choice", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: transcript.QuestionChoice, Options: choice("A", "B"), AllowCustom: true}}}},
		{name: "no fields", value: transcript.Question{}, wantErr: true},
		{name: "blank prompt", value: transcript.Question{Fields: []transcript.QuestionField{{Kind: transcript.QuestionText}}}, wantErr: true},
		{name: "long header", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Explain", Header: "1234567890123", Kind: transcript.QuestionText}}}, wantErr: true},
		{name: "text carries choice settings", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Explain", Kind: transcript.QuestionText, AllowCustom: true}}}, wantErr: true},
		{name: "choice has one option", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: transcript.QuestionChoice, Options: choice("A")}}}, wantErr: true},
		{name: "duplicate choice", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: transcript.QuestionChoice, Options: choice("A", "A")}}}, wantErr: true},
		{name: "padded choice", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: transcript.QuestionChoice, Options: choice(" A", "B")}}}, wantErr: true},
		{name: "unknown kind", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: transcript.QuestionFieldKind(99)}}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.value.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}

func TestApprovalValidateRequiresAPendingRiskClassifiedTool(t *testing.T) {
	result := tool.StringResult("already ran")
	tests := []struct {
		name     string
		approval transcript.Approval
		wantErr  bool
	}{
		{name: "pending", approval: transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell"}, Risk: tool.RiskHigh}},
		{name: "missing tool", approval: transcript.Approval{Risk: tool.RiskHigh}, wantErr: true},
		{name: "padded tool", approval: transcript.Approval{Tool: transcript.ToolInvocation{Name: " shell"}, Risk: tool.RiskHigh}, wantErr: true},
		{name: "missing risk", approval: transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell"}}, wantErr: true},
		{name: "completed result", approval: transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell", Result: &result}, Risk: tool.RiskHigh}, wantErr: true},
		{name: "offloaded result", approval: transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell", Offload: &toolresult.Ref{ID: "BLOB234"}}, Risk: tool.RiskHigh}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.approval.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}
