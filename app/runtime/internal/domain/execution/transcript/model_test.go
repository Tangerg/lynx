package transcript_test

import (
	"math"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestItemValidateOwnsPayloadInvariants(t *testing.T) {
	tests := []struct {
		name    string
		item    transcript.Item
		wantErr bool
	}{
		{
			name: "tool data on user message",
			item: transcript.Item{
				Kind: transcript.UserMessage,
				Tool: &transcript.ToolInvocation{Name: "shell"},
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.item.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}

func TestUsageAndProblemValidate(t *testing.T) {
	negativeCost := -0.1
	nanCost := math.NaN()
	infiniteCost := math.Inf(1)
	tests := []struct {
		name    string
		usage   *transcript.Usage
		problem *transcript.Problem
		wantErr bool
	}{
		{name: "valid"},
		{name: "negative token", usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{InputTokens: -1}}, wantErr: true},
		{name: "negative cost", usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{CostUSD: &negativeCost}}, wantErr: true},
		{name: "nan cost", usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{CostUSD: &nanCost}}, wantErr: true},
		{name: "infinite cost", usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{CostUSD: &infiniteCost}}, wantErr: true},
		{name: "unknown problem scope", problem: &transcript.Problem{Scope: transcript.ProblemScope(99)}, wantErr: true},
		{name: "wrong problem owner", problem: &transcript.Problem{Scope: transcript.RunProblem}, wantErr: true},
		{name: "retry delay on permanent failure", problem: &transcript.Problem{Scope: transcript.ToolProblem, Kind: transcript.InvalidAPIKeyProblem, RetryAfterSeconds: 1}, wantErr: true},
		{name: "retry delay on rate limit", problem: &transcript.Problem{Scope: transcript.ToolProblem, Kind: transcript.RateLimitedProblem, RetryAfterSeconds: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.usage.Validate()
			if err == nil && test.problem != nil {
				err = test.problem.ValidateFor(transcript.ToolProblem)
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
