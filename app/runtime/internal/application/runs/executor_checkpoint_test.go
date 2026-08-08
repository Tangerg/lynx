package runs

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func mustCheckpointSelection(provider, model string) modelref.Selection {
	selection, err := modelref.New(provider, model)
	if err != nil {
		panic(err)
	}
	return selection
}

func testExecutorCheckpoint() ExecutorCheckpoint {
	return ExecutorCheckpoint{
		RootMemberID:   "member_root",
		Payload:        []byte(`{"root":"member_root"}`),
		BuildID:        "build",
		Scope:          ExecutionScope{SessionID: "ses_1"},
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
		{name: "session", root: "member_root", session: "other_session", selection: mustCheckpointSelection("openai", "model")},
		{name: "goal lease", root: "member_root", session: "ses_1", goalLeaseID: "other_goal", selection: mustCheckpointSelection("openai", "model")},
		{name: "provider", root: "member_root", session: "ses_1", selection: mustCheckpointSelection("anthropic", "model")},
		{name: "model", root: "member_root", session: "ses_1", selection: mustCheckpointSelection("openai", "gpt-other")},
	} {
		t.Run(test.name, func(t *testing.T) {
			barrier := TreeInterrupted{
				Checkpoint: testExecutorCheckpoint(),
				Interruptions: []MemberInterruption{{
					MemberID:  "member_root",
					RequestID: "request_root",
					Interrupt: Interrupt{
						Kind: interrupt.Question,
						Question: &QuestionPrompt{
							ToolName:  "ask_user",
							Arguments: `{}`,
							Fields:    []QuestionFieldSpec{{Prompt: "Continue?", Header: "Continue"}},
						},
					},
				}},
			}
			if err := barrier.validateFor(test.root, test.session, test.goalLeaseID, test.selection); !errors.Is(err, ErrInvalidExecutorCheckpoint) {
				t.Fatalf("validateFor error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
		})
	}
}
