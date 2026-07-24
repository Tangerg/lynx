package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestArtifactFromPortableRejectsUnknownDomainEnums(t *testing.T) {
	tests := []struct {
		name     string
		portable sessions.PortableSnapshot
	}{
		{
			name: "outcome",
			portable: sessions.PortableSnapshot{Runs: []sessions.PortableRun{{
				ID: "run_1", Outcome: execution.Outcome(99),
			}}},
		},
		{
			name: "problem",
			portable: sessions.PortableSnapshot{Runs: []sessions.PortableRun{{
				ID: "run_1", Outcome: execution.OutcomeError,
				Result: &transcript.RunResult{Error: &transcript.Problem{Kind: transcript.ProblemKind(99)}},
			}}},
		},
		{
			name: "item status",
			portable: sessions.PortableSnapshot{Items: []transcript.Item{{
				ID: "item_1", Status: transcript.ItemStatus(99), Kind: transcript.UserMessage,
			}}},
		},
		{
			name: "item kind",
			portable: sessions.PortableSnapshot{Items: []transcript.Item{{
				ID: "item_1", Status: transcript.ItemCompleted, Kind: transcript.ItemKind(99),
			}}},
		},
		{
			name: "content kind",
			portable: sessions.PortableSnapshot{Items: []transcript.Item{{
				ID: "item_1", Status: transcript.ItemCompleted, Kind: transcript.AgentMessage,
				Content: []transcript.ContentBlock{{Kind: transcript.ContentKind(99)}},
			}}},
		},
		{
			name: "plan status",
			portable: sessions.PortableSnapshot{Items: []transcript.Item{{
				ID: "item_1", Status: transcript.ItemCompleted, Kind: transcript.Plan,
				Steps: []transcript.PlanStep{{Status: transcript.PlanStepStatus("future")}},
			}}},
		},
		{
			name: "question field kind",
			portable: sessions.PortableSnapshot{Items: []transcript.Item{{
				ID: "item_1", Status: transcript.ItemCompleted, Kind: transcript.QuestionItem,
				Question: &transcript.Question{Fields: []transcript.QuestionField{{Kind: transcript.QuestionFieldKind(99)}}},
			}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := artifactFromPortable(test.portable); err == nil {
				t.Fatal("artifact encoding accepted an unknown domain enum")
			}
		})
	}
}
