package memory

import (
	"context"
	"errors"
	"iter"
	"maps"

	"github.com/Tangerg/lynx/ai/model/chat"
)

const (
	// ConversationIDKey is the key used to store conversation ID in request parameters
	ConversationIDKey = "lynx:ai:model:chat:memory:conversation_id"

	// SavedMarkerKey is the key used to mark a message as saved in its metadata
	SavedMarkerKey = "lynx:ai:model:chat:memory:saved_marker"
)

// savedMarker is an empty struct used as a marker to indicate a message has been saved.
// Using an empty struct consumes zero memory while providing type safety.
type savedMarker struct{}

// chatMemoryMiddleware manages conversation history storage and retrieval.
// It prevents duplicate message storage and maintains conversation context.
type chatMemoryMiddleware struct {
	store Store
}

// NewMemoryMiddleware creates a new chat memory middleware with the given storage.
// Returns both synchronous call and streaming middleware implementations.
// Returns error if store is nil.
func NewMemoryMiddleware(store Store) (chat.CallMiddleware, chat.StreamMiddleware, error) {
	if store == nil {
		return nil, nil, errors.New("memory storage is required")
	}
	mw := &chatMemoryMiddleware{
		store: store,
	}
	return mw.wrapCallHandler, mw.wrapStreamHandler, nil
}

// extractConversationID retrieves conversation ID from request parameters.
// Returns empty string if conversation ID is not set.
// Returns error if conversation ID exists but is not a string.
func (m *chatMemoryMiddleware) extractConversationID(req *chat.Request) (string, error) {
	convID, exists := req.Get(ConversationIDKey)
	if !exists {
		return "", nil
	}

	id, ok := convID.(string)
	if !ok {
		return "", errors.New("conversation id must be a string")
	}

	return id, nil
}

// isMessageSaved checks if a message has been saved to memory by looking for the saved marker.
// Returns false if the message has no metadata or no saved marker.
func (m *chatMemoryMiddleware) isMessageSaved(msg chat.Message) bool {
	meta := msg.Meta()
	if meta == nil {
		return false
	}
	_, ok := meta[SavedMarkerKey].(savedMarker)
	return ok
}

// markMessageAsSaved marks a message as saved to memory by adding a saved marker to its metadata.
// Does nothing if the message has no metadata.
func (m *chatMemoryMiddleware) markMessageAsSaved(msg chat.Message) {
	meta := msg.Meta()
	if meta == nil {
		return
	}
	meta[SavedMarkerKey] = savedMarker{}
}

// filterUnsavedMessages filters out messages that have already been saved.
// Returns a new slice containing only unsaved messages.
func (m *chatMemoryMiddleware) filterUnsavedMessages(msgs []chat.Message) []chat.Message {
	var unsaved []chat.Message
	for _, msg := range msgs {
		if !m.isMessageSaved(msg) {
			unsaved = append(unsaved, msg)
		}
	}
	return unsaved
}

// retrieveHistoryMessages loads historical messages for the given conversation from storage.
// Marks all retrieved messages as saved to prevent re-saving them.
// Returns nil if no conversation ID is set.
func (m *chatMemoryMiddleware) retrieveHistoryMessages(ctx context.Context, req *chat.Request) ([]chat.Message, error) {
	id, err := m.extractConversationID(req)
	if err != nil {
		return nil, err
	}

	if id == "" {
		return nil, nil
	}

	history, err := m.store.Read(ctx, id)
	if err != nil {
		return nil, err
	}

	// Mark all history messages as saved to prevent duplicate storage
	for _, msg := range history {
		m.markMessageAsSaved(msg)
	}

	return history, nil
}

// persistMessages saves messages to the conversation history storage.
// Only saves messages that haven't been saved before (checked by saved marker).
// Marks successfully saved messages with the saved marker.
// Does nothing if no conversation ID is set or no messages to save.
func (m *chatMemoryMiddleware) persistMessages(ctx context.Context, req *chat.Request, msgs ...chat.Message) error {
	id, err := m.extractConversationID(req)
	if err != nil {
		return err
	}

	if id == "" || len(msgs) == 0 {
		return nil
	}

	err = m.store.Write(ctx, id, msgs...)
	if err != nil {
		return err
	}

	// Mark messages as saved after successful write
	for _, msg := range msgs {
		m.markMessageAsSaved(msg)
	}

	return nil
}

// prepareRequest prepares the request by:
// 1. Loading historical messages from storage
// 2. Filtering and saving new incoming messages
// 3. Combining history with new messages
// 4. Creating a new request with full conversation context
func (m *chatMemoryMiddleware) prepareRequest(ctx context.Context, req *chat.Request) (*chat.Request, error) {
	// Load historical messages from storage
	history, err := m.retrieveHistoryMessages(ctx, req)
	if err != nil {
		return nil, err
	}

	// Filter out messages that have already been saved (e.g., when user passes history)
	newMsgs := m.filterUnsavedMessages(req.Messages)

	// Save new messages to storage
	if len(newMsgs) > 0 {
		err = m.persistMessages(ctx, req, newMsgs...)
		if err != nil {
			return nil, err
		}
	}

	// Combine history with new messages to form complete conversation context
	allMsgs := append(history, newMsgs...)

	// Create new request with full context
	newReq, err := chat.NewRequest(allMsgs)
	if err != nil {
		return nil, err
	}

	// Preserve original request options and parameters
	newReq.Options = req.Options.Clone()
	newReq.Params = maps.Clone(req.Params)

	return newReq, nil
}

// saveResponseMessages persists assistant and tool messages from the AI response.
// AI-generated messages are guaranteed to be new, so no filtering is needed.
func (m *chatMemoryMiddleware) saveResponseMessages(ctx context.Context, req *chat.Request, resp *chat.Response) error {
	var msgs []chat.Message

	// Collect all assistant messages from results
	for _, result := range resp.Results {
		msgs = append(msgs, result.AssistantMessage)
	}

	// Collect tool messages if present
	for _, result := range resp.Results {
		if result.ToolMessage != nil {
			msgs = append(msgs, result.ToolMessage)
		}
	}

	// Save AI-generated messages (no filtering needed as they are all new)
	return m.persistMessages(ctx, req, msgs...)
}

// executeCall handles synchronous call with memory management.
// Workflow:
// 1. Prepare request with conversation history
// 2. Call next handler with prepared request
// 3. Save AI response messages to memory
func (m *chatMemoryMiddleware) executeCall(ctx context.Context, req *chat.Request, next chat.CallHandler) (*chat.Response, error) {
	newReq, err := m.prepareRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := next.Call(ctx, newReq)
	if err != nil {
		return nil, err
	}

	err = m.saveResponseMessages(ctx, req, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// executeStream handles streaming call with memory management.
// Workflow:
// 1. Prepare request with conversation history
// 2. Stream responses from next handler
// 3. Accumulate streaming chunks
// 4. Save complete AI response to memory after streaming completes
func (m *chatMemoryMiddleware) executeStream(ctx context.Context, req *chat.Request, next chat.StreamHandler) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		newReq, err := m.prepareRequest(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}

		// Accumulate streaming chunks to get the complete response
		acc := chat.NewResponseAccumulator()

		for resp, err := range next.Stream(ctx, newReq) {
			if err != nil {
				yield(nil, err)
				return
			}

			acc.AddChunk(resp)

			// Yield chunk to caller
			if !yield(resp, nil) {
				break
			}
		}

		// Save complete response after streaming finishes
		err = m.saveResponseMessages(ctx, req, &acc.Response)
		if err != nil {
			yield(nil, err)
		}
	}
}

// wrapCallHandler wraps a call handler with memory middleware functionality.
func (m *chatMemoryMiddleware) wrapCallHandler(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

// wrapStreamHandler wraps a stream handler with memory middleware functionality.
func (m *chatMemoryMiddleware) wrapStreamHandler(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
}
