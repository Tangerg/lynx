package transcript_test

import (
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
			name: "valid plan",
			item: transcript.Item{
				Kind:  transcript.Plan,
				Steps: []transcript.PlanStep{{Status: transcript.PlanStepPending}},
			},
		},
		{
			name: "unknown plan status",
			item: transcript.Item{
				Kind:  transcript.Plan,
				Steps: []transcript.PlanStep{{Status: transcript.PlanStepStatus("stalled")}},
			},
			wantErr: true,
		},
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
	tests := []struct {
		name    string
		usage   *transcript.Usage
		problem *transcript.Problem
		wantErr bool
	}{
		{name: "valid"},
		{name: "negative token", usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{InputTokens: -1}}, wantErr: true},
		{name: "negative cost", usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{CostUSD: &negativeCost}}, wantErr: true},
		{name: "unknown problem scope", problem: &transcript.Problem{Scope: transcript.ProblemScope(99)}, wantErr: true},
		{name: "wrong problem owner", problem: &transcript.Problem{Scope: transcript.RunProblem}, wantErr: true},
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
