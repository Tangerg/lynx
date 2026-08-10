// Package itemfixture provides test-only construction helpers for valid
// transcript Items. Production code must use the semantic Domain constructors.
package itemfixture

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// Input mirrors the facts old fixtures commonly spell out while keeping the
// Domain Item itself private. It is intentionally test-only and is not a second
// production representation.
type Input struct {
	SessionID       string
	RunID           string
	ID              string
	Status          transcript.ItemStatus
	FinishedAt      time.Time
	Kind            transcript.ItemKind
	OccurredAt      time.Time
	Content         []transcript.ContentBlock
	Text            string
	Redacted        bool
	Question        *transcript.Question
	Tool            *transcript.ToolInvocation
	SafetyClass     tool.SafetyClass
	Failure         *tool.Failure
	Summary         string
	DroppedMessages int
}

// MustRestore returns one valid Item or panics. Tests exercising invalid
// snapshots must call transcript.RestoreItem directly and assert the error.
func MustRestore(input Input) transcript.Item {
	if input.SessionID == "" {
		input.SessionID = "session_fixture"
	}
	if input.RunID == "" {
		input.RunID = "run_fixture"
	}
	if input.ID == "" {
		input.ID = "item_fixture"
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = time.Unix(1, 0).UTC()
	}
	switch input.Kind {
	case transcript.UserMessage, transcript.AgentMessage:
		input.Status = transcript.ItemCompleted
		if len(input.Content) == 0 {
			input.Content = []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "fixture"}}
		}
	case transcript.Reasoning:
		input.Status = transcript.ItemCompleted
		if input.Text == "" {
			input.Text = "fixture"
		}
	case transcript.QuestionItem:
		input.Status = transcript.ItemCompleted
		if input.Question == nil {
			input.Question = &transcript.Question{Fields: []transcript.QuestionField{{
				Prompt: "fixture", Kind: transcript.QuestionText,
			}}}
		}
	case transcript.Compaction:
		input.Status = transcript.ItemCompleted
	case transcript.ToolCall:
		if input.Tool == nil {
			input.Tool = &transcript.ToolInvocation{Name: "fixture"}
		}
		if input.Status != transcript.ItemRunning && input.FinishedAt.IsZero() {
			input.FinishedAt = input.OccurredAt
		}
	}
	item, err := transcript.RestoreItem(transcript.ItemSnapshot{
		Identity: transcript.ItemIdentity{
			SessionID: input.SessionID, RunID: input.RunID, ItemID: input.ID,
			OccurredAt: input.OccurredAt,
		},
		Status: input.Status, FinishedAt: input.FinishedAt, Kind: input.Kind,
		Content: input.Content, Text: input.Text, Redacted: input.Redacted,
		Question: input.Question, Tool: input.Tool, SafetyClass: input.SafetyClass,
		Failure: input.Failure, Summary: input.Summary, DroppedMessages: input.DroppedMessages,
	})
	if err != nil {
		panic(err)
	}
	return item
}
