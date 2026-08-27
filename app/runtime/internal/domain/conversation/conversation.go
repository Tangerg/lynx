// Package conversation owns the model-context message sequence independently
// from Runs, transcript observations, and executor working state.
package conversation

import (
	"errors"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
)

var (
	// ErrInvalid reports a malformed conversation value.
	ErrInvalid = errors.New("conversation: invalid")
	// ErrNotEmpty reports an attempt to seed a conversation that already has
	// messages. Seed is a fork/import operation, never append under another name.
	ErrNotEmpty = errors.New("conversation: seed requires an empty conversation")
)

// Conversation is an ownership-isolated model-context sequence. It contains no
// persistence, Run, transcript, or executor state.
type Conversation struct {
	messages []chat.Message
}

// New validates and owns messages as one conversation value.
func New(messages []chat.Message) (Conversation, error) {
	owned := make([]chat.Message, len(messages))
	for index, message := range messages {
		if err := message.Validate(); err != nil {
			return Conversation{}, fmt.Errorf("%w: message[%d]: %w", ErrInvalid, index, err)
		}
		owned[index] = message.Clone()
	}
	return Conversation{messages: owned}, nil
}

// Messages returns an ownership-isolated snapshot in model-context order.
func (c Conversation) Messages() []chat.Message {
	out := make([]chat.Message, len(c.messages))
	for index, message := range c.messages {
		out[index] = message.Clone()
	}
	return out
}

// Count returns the current message watermark.
func (c Conversation) Count() int { return len(c.messages) }

// Seed replaces an empty conversation with a validated fork/import prefix.
func (c Conversation) Seed(messages []chat.Message) (Conversation, error) {
	if len(c.messages) != 0 {
		return Conversation{}, ErrNotEmpty
	}
	return New(messages)
}

// Append returns the conversation extended by messages in model-context order.
func (c Conversation) Append(messages ...chat.Message) (Conversation, error) {
	combined := make([]chat.Message, 0, len(c.messages)+len(messages))
	combined = append(combined, c.messages...)
	combined = append(combined, messages...)
	return New(combined)
}

// CloseOpenToolCalls returns the conversation with one error result appended
// for every provider ToolCall that has no later ToolResult. The results are
// ordered by the calls' first unresolved occurrence and share one Tool message,
// matching the provider-neutral conversation protocol. A resolved call ID may
// be reused by a later generation without reopening the former generation.
func (c Conversation) CloseOpenToolCalls(result string) (Conversation, []chat.Message, error) {
	return c.CloseOpenToolCallsWithResults(result, nil)
}

// CloseOpenToolCallsWithResults is CloseOpenToolCalls with authoritative
// results for calls that completed out of order but could not yet be appended.
// Every supplied result must match the latest unresolved generation. The final
// Tool message follows provider call order and fills only the remaining calls
// with the fallback error, so a terminal boundary preserves known output
// without leaving a malformed model history.
func (c Conversation) CloseOpenToolCallsWithResults(
	result string,
	completed []chat.ToolResult,
) (Conversation, []chat.Message, error) {
	type openCall struct {
		call       chat.ToolCall
		generation int
	}
	ordered := make([]openCall, 0)
	open := make(map[string]int)
	generation := 0
	for _, message := range c.messages {
		for _, part := range message.Parts {
			if call := part.ToolCall; call != nil {
				if _, duplicate := open[call.ID]; !duplicate {
					generation++
					open[call.ID] = generation
					ordered = append(ordered, openCall{call: *call, generation: generation})
				}
			}
			if toolResult := part.ToolResult; toolResult != nil {
				delete(open, toolResult.ID)
			}
		}
	}
	known := make(map[string]chat.ToolResult, len(completed))
	for _, toolResult := range completed {
		if _, duplicate := known[toolResult.ID]; duplicate {
			return Conversation{}, nil, fmt.Errorf(
				"%w: completed ToolResult %q is repeated",
				ErrInvalid,
				toolResult.ID,
			)
		}
		known[toolResult.ID] = toolResult
	}
	if len(open) == 0 && len(known) == 0 {
		return c, nil, nil
	}
	results := make([]chat.ToolResult, 0, len(open))
	for _, unresolved := range ordered {
		current, present := open[unresolved.call.ID]
		if !present || current != unresolved.generation {
			continue
		}
		if completedResult, ok := known[unresolved.call.ID]; ok {
			if completedResult.Name != unresolved.call.Name {
				return Conversation{}, nil, fmt.Errorf(
					"%w: completed ToolResult %q names %q, want %q",
					ErrInvalid,
					completedResult.ID,
					completedResult.Name,
					unresolved.call.Name,
				)
			}
			results = append(results, completedResult)
			delete(known, unresolved.call.ID)
			continue
		}
		results = append(results, chat.ToolResult{
			ID: unresolved.call.ID, Name: unresolved.call.Name,
			Result: result, IsError: true,
		})
	}
	if len(known) != 0 {
		for id := range known {
			return Conversation{}, nil, fmt.Errorf(
				"%w: completed ToolResult %q has no unresolved ToolCall",
				ErrInvalid,
				id,
			)
		}
	}
	if len(results) == 0 {
		return c, nil, nil
	}
	appended := []chat.Message{chat.NewToolMessage(results...)}
	closed, err := c.Append(appended...)
	if err != nil {
		return Conversation{}, nil, err
	}
	return closed, appended, nil
}

// Truncate returns the prefix ending at keepN. Values below zero clear the
// conversation; values beyond the current watermark are a no-op.
func (c Conversation) Truncate(keepN int) Conversation {
	keepN = max(keepN, 0)
	keepN = min(keepN, len(c.messages))
	truncated, _ := New(c.messages[:keepN])
	return truncated
}
