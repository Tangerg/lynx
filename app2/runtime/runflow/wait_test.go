package runflow

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestWaitingProjectionRejectsInvalidQuestionTree(t *testing.T) {
	prompt := agentexec.ToolInputPrompt{
		Kind: "question", ItemID: "item_question",
		Question: &protocol.Question{Fields: []protocol.QuestionField{{
			Prompt: "Pick one", Type: protocol.QuestionFieldChoice,
			Options: []protocol.QuestionOption{{Label: "Only"}},
		}}},
	}

	_, _, err := waitingProjection("run_test", prompt, time.Now())
	if err == nil || !strings.Contains(err.Error(), "options") {
		t.Fatalf("waitingProjection() error = %v, want nested options constraint", err)
	}
}

func TestWaitingProjectionProducesWireValidQuestion(t *testing.T) {
	prompt := agentexec.ToolInputPrompt{
		Kind: "question", ItemID: "item_question",
		Question: &protocol.Question{Fields: []protocol.QuestionField{{
			Prompt: "Pick one", Type: protocol.QuestionFieldChoice,
			Options: []protocol.QuestionOption{{Label: "Blue"}, {Label: "Green"}},
		}}},
	}

	item, interrupt, err := waitingProjection("run_test", prompt, time.Now())
	if err != nil {
		t.Fatalf("waitingProjection() error = %v", err)
	}
	if err := protocol.ValidateWireTree(item); err != nil {
		t.Fatalf("projected Item is invalid: %v", err)
	}
	if err := protocol.ValidateWireTree(interrupt); err != nil {
		t.Fatalf("projected Interrupt is invalid: %v", err)
	}
}
