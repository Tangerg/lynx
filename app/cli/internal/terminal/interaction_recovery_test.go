package terminal

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/agent/mock"
)

type invalidEventAfterInterruptRuntime struct {
	agent.Runtime
	release <-chan struct{}
}

func (i invalidEventAfterInterruptRuntime) StartRun(
	ctx context.Context,
	command agent.StartRun,
) (agent.SegmentStream, error) {
	stream, err := i.Runtime.StartRun(ctx, command)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	original := stream.Events
	stream.Events = func(yield func(agent.RunEvent, error) bool) {
		for event, streamErr := range original {
			if !yield(event, streamErr) {
				return
			}
		}
		select {
		case <-i.release:
		case <-ctx.Done():
			return
		}
		yield(agent.RunEvent{
			EventID: "evt_invalid_after_interrupt", RunID: stream.RunID, SegmentID: stream.SegmentID,
			At: time.Now(), Event: agent.BlockDelta{BlockID: "missing", Text: "invalid tail"},
		}, nil)
	}
	return stream, nil
}

func TestStreamFailureRetiresTheObsoleteInteractionProjection(t *testing.T) {
	approval := func(arguments bool) agent.Approval {
		call := &agent.ToolCall{
			Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning,
		}
		if arguments {
			call.ArgumentsJSON = []byte(`{"command":"go test ./..."}`)
		}
		return agent.Approval{ItemID: "approval_before_stream_failure", Title: "Approve before failure", Tool: call}
	}
	tests := []struct {
		name        string
		interaction agent.Interaction
		open        string
		editArgs    bool
		obsolete    []string
	}{
		{
			name: "approval", interaction: approval(false), open: "Tool approval",
			obsolete: []string{"Tool approval"},
		},
		{
			name: "approval argument editor", interaction: approval(true), open: "Tool approval",
			editArgs: true,
			obsolete: []string{"Tool approval", "Edit tool arguments"},
		},
		{
			name: "question",
			interaction: agent.Question{
				ItemID: "question_before_stream_failure", Title: "Choose before failure",
				Fields: []agent.QuestionField{{
					Header: "Strategy", Prompt: "Choose a strategy", Kind: agent.QuestionSingle,
					Options: []agent.QuestionOption{{Label: "Safe"}, {Label: "Fast"}},
				}},
			},
			open: "Choose before failure", obsolete: []string{"Safe"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := mock.New()
			backend.Instant = true
			backend.Script = func(string) mock.Script {
				return mock.Script{
					Interactions: []agent.Interaction{test.interaction},
					Continue: func([]agent.InterruptAnswer) []mock.Step {
						return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
					},
				}
			}
			release := make(chan struct{})
			host, stop := runUIWith(t, invalidEventAfterInterruptRuntime{Runtime: backend, release: release})
			host.Shows(t, "Ask lyra")
			host.Type("exercise an invalid event after HITL")
			host.Press(input.Enter)
			host.Shows(t, test.open)
			if test.editArgs {
				host.Press(input.End)
				host.Press(input.Enter)
				host.Shows(t, "Edit tool arguments")
			}
			close(release)
			host.Shows(t, "apply runtime event evt_invalid_after_interrupt")
			for _, obsolete := range test.obsolete {
				host.Hides(t, obsolete)
			}
			host.Type("/plugins")
			host.Press(input.Enter)
			host.Shows(t, "terminal.core@1.0.0")
			stop()
		})
	}
}

func TestPendingResumePersistenceFailureReopensTheInteractionForRetry(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.ApprovalAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "approval_resume_persistence", Title: "Persist before resuming",
				Tool: &agent.ToolCall{
					Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning,
				},
			}},
			Continue: func(provided []agent.InterruptAnswer) []mock.Step {
				answers <- provided[0].Answer.(agent.ApprovalAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	stateDirectory := t.TempDir()
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, Workspace: t.TempDir(), StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	host.Type("exercise local resume persistence")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")

	restoreStateDirectory := blockStateDirectoryWrites(t, stateDirectory)
	host.Press(input.Enter)
	host.Shows(t, "resume blocked: save interaction decisions")
	host.Shows(t, "Tool approval")
	select {
	case answer := <-answers:
		t.Fatalf("runtime resumed without a durable command: %+v", answer)
	default:
	}

	restoreStateDirectory()
	host.Press(input.Enter)
	host.Shows(t, "complete")
	if answer := <-answers; answer.Decision != agent.ApprovalApprove {
		t.Fatalf("retried approval answer = %+v", answer)
	}
	stop()
}

func TestPendingResumePersistenceFailureReopensTheQuestionForRetry(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.QuestionAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Question{
				ItemID: "question_resume_persistence", Title: "Persist question before resuming",
				Fields: []agent.QuestionField{{
					Prompt: "Strategy", Kind: agent.QuestionSingle,
					Options: []agent.QuestionOption{{Label: "Safe"}, {Label: "Fast"}},
				}},
			}},
			Continue: func(provided []agent.InterruptAnswer) []mock.Step {
				answers <- provided[0].Answer.(agent.QuestionAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	stateDirectory := t.TempDir()
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, Workspace: t.TempDir(), StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	host.Type("exercise question resume persistence")
	host.Press(input.Enter)
	host.Shows(t, "Persist question before resuming")

	restoreStateDirectory := blockStateDirectoryWrites(t, stateDirectory)
	host.Press(input.Enter)
	host.Shows(t, "resume blocked: save interaction decisions")
	host.Shows(t, "Persist question before resuming")
	host.Shows(t, "Safe")
	select {
	case answer := <-answers:
		t.Fatalf("runtime resumed without a durable command: %+v", answer)
	default:
	}

	restoreStateDirectory()
	host.Press(input.Enter)
	host.Shows(t, "complete")
	answer := <-answers
	if len(answer.Values) != 1 || !slices.Equal(answer.Values[0], []string{"Safe"}) {
		t.Fatalf("retried question answer = %+v", answer)
	}
	stop()
}

func TestPendingResumePersistenceFailureReopensTheBatchReviewForRetry(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan []agent.InterruptAnswer, 1)
	backend.Script = func(string) mock.Script { return multiInteractionReviewScript(answers) }
	stateDirectory := t.TempDir()
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, Workspace: t.TempDir(), StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	host.Type("exercise batch resume persistence")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Press(input.Enter)
	host.Shows(t, "Choose platform")
	host.Press(input.Enter)
	host.Shows(t, "Review interactions")

	restoreStateDirectory := blockStateDirectoryWrites(t, stateDirectory)
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "resume blocked: save interaction decisions")
	host.Shows(t, "Review interactions")
	select {
	case answer := <-answers:
		t.Fatalf("runtime resumed without a durable command: %+v", answer)
	default:
	}

	restoreStateDirectory()
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "complete")
	if answer := <-answers; len(answer) != 2 {
		t.Fatalf("retried interaction batch = %+v", answer)
	}
	stop()
}

func blockStateDirectoryWrites(t *testing.T, stateDirectory string) func() {
	t.Helper()
	if err := os.RemoveAll(stateDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateDirectory, []byte("block durable state writes"), 0o600); err != nil {
		t.Fatal(err)
	}
	blocked := true
	restore := func() {
		t.Helper()
		if !blocked {
			return
		}
		blocked = false
		if err := os.RemoveAll(stateDirectory); err != nil {
			t.Error(err)
			return
		}
		if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(restore)
	return restore
}
