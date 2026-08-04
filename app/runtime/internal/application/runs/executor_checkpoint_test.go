package runs

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func mustCheckpointSelection(provider, model string) modelref.Selection {
	selection, err := modelref.New(provider, model)
	if err != nil {
		panic(err)
	}
	return selection
}

func testExecutorCheckpoint() execution.ExecutorCheckpoint {
	return execution.ExecutorCheckpoint{
		RootProcessID:  "process_root",
		Payload:        []byte(`{"root":"process_root"}`),
		BuildID:        "build",
		Scope:          execution.ExecutionScope{SessionID: "ses_1"},
		ModelSelection: mustCheckpointSelection("openai", "model"),
	}
}

func TestTreeInterruptedRejectsCheckpointBoundToDifferentApplicationFacts(t *testing.T) {
	for _, test := range []struct {
		name        string
		root        string
		session     string
		goalLeaseID string
		selection   modelref.Selection
	}{
		{name: "root", root: "other_root", session: "ses_1", selection: mustCheckpointSelection("openai", "model")},
		{name: "session", root: "process_root", session: "other_session", selection: mustCheckpointSelection("openai", "model")},
		{name: "goal lease", root: "process_root", session: "ses_1", goalLeaseID: "other_goal", selection: mustCheckpointSelection("openai", "model")},
		{name: "provider", root: "process_root", session: "ses_1", selection: mustCheckpointSelection("anthropic", "model")},
		{name: "model", root: "process_root", session: "ses_1", selection: mustCheckpointSelection("openai", "gpt-other")},
	} {
		t.Run(test.name, func(t *testing.T) {
			barrier := TreeInterrupted{
				Checkpoint: testExecutorCheckpoint(),
				Suspensions: []ProcessSuspension{{
					ProcessID:    "process_root",
					SuspensionID: "suspension_root",
					Interrupt: Interrupt{
						Kind: execution.QuestionInterrupt,
						Question: &QuestionPrompt{
							ToolName:  "ask_user",
							Arguments: `{}`,
							Fields:    []QuestionFieldSpec{{Prompt: "Continue?", Header: "Continue"}},
						},
					},
				}},
			}
			if err := barrier.validateFor(test.root, test.session, test.goalLeaseID, test.selection); !errors.Is(err, execution.ErrInvalidExecutorCheckpoint) {
				t.Fatalf("validateFor error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
		})
	}
}
