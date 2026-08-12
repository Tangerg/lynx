package transcript_test

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func itemIdentity() transcript.ItemIdentity {
	location := time.FixedZone("fixture", 8*60*60)
	return transcript.ItemIdentity{
		SessionID:  "session-1",
		RunID:      "run-1",
		ItemID:     "item-1",
		OccurredAt: time.Date(2026, 8, 10, 12, 0, 0, 0, location),
	}
}

func TestItemConstructorsCloseEveryVariant(t *testing.T) {
	identity := itemIdentity()
	constructors := []struct {
		name string
		kind transcript.ItemKind
		new  func() (transcript.Item, error)
	}{
		{
			name: "user message", kind: transcript.UserMessage,
			new: func() (transcript.Item, error) {
				return transcript.NewUserMessage(identity, []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}})
			},
		},
		{
			name: "agent message", kind: transcript.AgentMessage,
			new: func() (transcript.Item, error) {
				return transcript.NewAgentMessage(identity, []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}})
			},
		},
		{
			name: "reasoning", kind: transcript.Reasoning,
			new: func() (transcript.Item, error) {
				return transcript.NewReasoning(identity, "consider the evidence", true)
			},
		},
		{
			name: "question", kind: transcript.QuestionItem,
			new: func() (transcript.Item, error) {
				return transcript.NewQuestion(identity, transcript.Question{Fields: []transcript.QuestionField{{
					Prompt: "Continue?", Kind: transcript.QuestionText,
				}}})
			},
		},
		{
			name: "compaction", kind: transcript.Compaction,
			new: func() (transcript.Item, error) {
				return transcript.NewCompaction(identity, "summary", 12)
			},
		},
	}
	for _, constructor := range constructors {
		t.Run(constructor.name, func(t *testing.T) {
			item, err := constructor.new()
			if err != nil {
				t.Fatalf("construct Item: %v", err)
			}
			if item.Kind() != constructor.kind || item.Status() != transcript.ItemCompleted {
				t.Fatalf("Item kind/status = %v/%v, want %v/completed", item.Kind(), item.Status(), constructor.kind)
			}
			if item.OccurredAt().Location() != time.UTC {
				t.Fatalf("occurrence location = %v, want UTC", item.OccurredAt().Location())
			}
		})
	}
}

func TestItemOwnsMutablePayloads(t *testing.T) {
	image := []byte{1, 2, 3}
	message, err := transcript.NewUserMessage(itemIdentity(), []transcript.ContentBlock{{
		Kind: transcript.ImageContent, MediaType: "image/png", Bytes: image,
	}})
	if err != nil {
		t.Fatalf("NewUserMessage: %v", err)
	}
	image[0] = 9
	content := message.Content()
	content[0].Bytes[1] = 8
	if got := message.Content()[0].Bytes; got[0] != 1 || got[1] != 2 {
		t.Fatalf("message shares mutable image bytes: %v", got)
	}

	questionInput := transcript.Question{Fields: []transcript.QuestionField{{
		Prompt: "Choose", Kind: transcript.QuestionChoice,
		Options: []transcript.QuestionOption{{Label: "A"}, {Label: "B"}},
	}}}
	questionItem, err := transcript.NewQuestion(itemIdentity(), questionInput)
	if err != nil {
		t.Fatalf("NewQuestion: %v", err)
	}
	questionInput.Fields[0].Options[0].Label = "changed"
	question, _ := questionItem.Question()
	question.Fields[0].Options[1].Label = "changed"
	owned, _ := questionItem.Question()
	if owned.Fields[0].Options[0].Label != "A" || owned.Fields[0].Options[1].Label != "B" {
		t.Fatalf("question shares mutable option storage: %+v", owned)
	}
}

func TestAnswerQuestionEnrichesAnImmutablePromptExactlyOnce(t *testing.T) {
	prompt, err := transcript.NewQuestion(itemIdentity(), transcript.Question{
		Fields: []transcript.QuestionField{{
			Prompt: "Choose", Kind: transcript.QuestionChoice,
			Options: []transcript.QuestionOption{{Label: "A"}, {Label: "B"}},
		}},
	})
	if err != nil {
		t.Fatalf("NewQuestion: %v", err)
	}
	answers := [][]string{{"B"}}
	answered, err := prompt.AnswerQuestion(answers)
	if err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	answers[0][0] = "A"
	original, _ := prompt.Question()
	accepted, _ := answered.Question()
	if original.Answered() || !accepted.Answered() || accepted.Answers[0][0] != "B" {
		t.Fatalf("prompt/answered = %+v / %+v", original, accepted)
	}
	accepted.Answers[0][0] = "A"
	again, _ := answered.Question()
	if again.Answers[0][0] != "B" {
		t.Fatalf("answered Question shares returned answer storage: %+v", again)
	}
	if _, err := answered.AnswerQuestion([][]string{{"A"}}); err == nil {
		t.Fatal("AnswerQuestion accepted a second answer")
	}
}

func TestItemForkReidentifiesTerminalHistoryAndRemapsOffload(t *testing.T) {
	running, err := transcript.NewToolCall(
		itemIdentity(), transcript.ToolInvocation{Name: "read_large"}, tool.SafetyClassSafe,
	)
	if err != nil {
		t.Fatalf("NewToolCall: %v", err)
	}
	if _, err := running.Fork("session-child", "run-child", "item-child", nil); err == nil {
		t.Fatal("Fork accepted a running Item")
	}
	preview := tool.StringResult("preview")
	completed, err := running.CompleteToolCall(transcript.ToolInvocation{
		Name: "read_large", Result: &preview, Offload: &toolresult.Ref{ID: "SOURCE23"},
	}, running.OccurredAt(), running.OccurredAt().Add(time.Second))
	if err != nil {
		t.Fatalf("CompleteToolCall: %v", err)
	}
	if _, err := completed.Fork("session-child", "run-child", "item-child", nil); err == nil {
		t.Fatal("Fork removed an existing offload reference")
	}
	forked, err := completed.Fork(
		"session-child", "run-child", "item-child", &toolresult.Ref{ID: "TARGET23"},
	)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	invocation, present := forked.ToolInvocation()
	if forked.SessionID() != "session-child" || forked.RunID() != "run-child" ||
		forked.ID() != "item-child" || !present || invocation.Offload == nil ||
		invocation.Offload.ID != "TARGET23" {
		t.Fatalf("forked Item = %+v", forked.Snapshot())
	}
	sourceInvocation, _ := completed.ToolInvocation()
	if completed.SessionID() != "session-1" || completed.ID() != "item-1" ||
		sourceInvocation.Offload == nil || sourceInvocation.Offload.ID != "SOURCE23" {
		t.Fatalf("fork mutated source Item: %+v", completed.Snapshot())
	}
}

func TestToolCallSettlementIsTerminalAndPreservesIdentity(t *testing.T) {
	arguments, err := tool.ParseArguments(`{"path":"README.md"}`)
	if err != nil {
		t.Fatalf("ParseArguments: %v", err)
	}
	running, err := transcript.NewToolCall(
		itemIdentity(),
		transcript.ToolInvocation{Name: "read_file", Arguments: arguments},
		tool.SafetyClassSafe,
	)
	if err != nil {
		t.Fatalf("NewToolCall: %v", err)
	}
	if running.Status() != transcript.ItemRunning || !running.FinishedAt().IsZero() {
		t.Fatalf("running ToolCall = status %v, finished %v", running.Status(), running.FinishedAt())
	}

	finishedAt := running.OccurredAt().Add(time.Second)
	executionStartedAt := running.OccurredAt().Add(250 * time.Millisecond)
	result := tool.StringResult("contents")
	settlement := transcript.ToolInvocation{Name: "read_file", Arguments: arguments, Result: &result}
	completed, err := running.CompleteToolCall(settlement, executionStartedAt, finishedAt)
	if err != nil {
		t.Fatalf("CompleteToolCall: %v", err)
	}
	if completed.Status() != transcript.ItemCompleted || !completed.FinishedAt().Equal(finishedAt) {
		t.Fatalf("completed ToolCall = status %v, finished %v", completed.Status(), completed.FinishedAt())
	}
	duration, known := completed.ExecutionDuration()
	if !known || duration != 750*time.Millisecond {
		t.Fatalf("completed execution duration = %v/%t", duration, known)
	}
	if _, err := completed.FailToolCall(
		settlement, tool.Failure{Kind: tool.FailureExecution}, executionStartedAt, finishedAt,
	); err == nil {
		t.Fatal("terminal ToolCall accepted a second settlement")
	}

	if _, err := running.CompleteToolCall(
		transcript.ToolInvocation{Name: "other_tool", Arguments: arguments, Result: &result},
		executionStartedAt,
		finishedAt,
	); err == nil {
		t.Fatal("ToolCall settlement changed invocation identity")
	}
}

func TestToolCallFailureAndAbandonmentHaveOneWayTransitions(t *testing.T) {
	running, err := transcript.NewToolCall(
		itemIdentity(), transcript.ToolInvocation{Name: "shell"}, tool.SafetyClassExec,
	)
	if err != nil {
		t.Fatalf("NewToolCall: %v", err)
	}
	finishedAt := running.OccurredAt().Add(time.Second)
	executionStartedAt := running.OccurredAt().Add(500 * time.Millisecond)
	failure := tool.Failure{Kind: tool.FailureDenied, Detail: "not approved"}
	failed, err := running.FailToolCall(
		transcript.ToolInvocation{Name: "shell"}, failure, executionStartedAt, finishedAt,
	)
	if err != nil {
		t.Fatalf("FailToolCall: %v", err)
	}
	gotFailure, present := failed.Failure()
	if failed.Status() != transcript.ItemIncomplete || !present || gotFailure != failure {
		t.Fatalf("failed ToolCall = status %v, failure %+v/%t", failed.Status(), gotFailure, present)
	}
	if _, err := failed.ClassifyAbandonedToolCall(failure); err == nil {
		t.Fatal("already-classified ToolCall accepted another failure")
	}

	abandoned, err := running.AbandonToolCall(nil, finishedAt)
	if err != nil {
		t.Fatalf("AbandonToolCall: %v", err)
	}
	if duration, known := abandoned.ExecutionDuration(); known {
		t.Fatalf("unstarted abandonment execution duration = %v", duration)
	}
	causal := tool.Failure{Kind: tool.FailureChildRunCanceled, Detail: "child canceled"}
	classified, err := abandoned.ClassifyAbandonedToolCall(causal)
	if err != nil {
		t.Fatalf("ClassifyAbandonedToolCall: %v", err)
	}
	gotFailure, present = classified.Failure()
	if !present || gotFailure != causal || !classified.FinishedAt().Equal(abandoned.FinishedAt()) {
		t.Fatalf("classified abandoned ToolCall = failure %+v/%t, finished %v", gotFailure, present, classified.FinishedAt())
	}
}
