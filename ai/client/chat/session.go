package chat

import (
	"errors"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/tool"
)

type Session struct {
	chatModel            chat.Model
	chatOptions          chat.Options
	userPromptTemplate   *PromptTemplate
	systemPromptTemplate *PromptTemplate
	messages             []messages.Message
	middlewareManager    *MiddlewareManager
	params               map[string]any
	tools                []tool.Tool
	toolParams           map[string]any
}

func NewSession(chatModel chat.Model) (*Session, error) {
	if chatModel == nil {
		return nil, errors.New("chatModel is required")
	}

	return &Session{
		chatModel:         chatModel,
		middlewareManager: NewMiddlewareManager(),
		params:            make(map[string]any),
		tools:             make([]tool.Tool, 0),
		toolParams:        make(map[string]any),
	}, nil
}

func (s *Session) Call() *Caller {
	caller, _ := NewCaller(s)
	return caller
}

func (s *Session) Stream() *Streamer {
	streamer, _ := NewStreamer(s)
	return streamer
}

func (s *Session) WithChatOptions(chatOptions chat.Options) *Session {
	if chatOptions != nil {
		s.chatOptions = chatOptions.Clone()
	}
	return s
}

func (s *Session) WithUserPrompt(userPrompt string) *Session {
	if userPrompt != "" {
		s.userPromptTemplate = NewPromptTemplate().WithTemplate(userPrompt)
	}
	return s
}

func (s *Session) WithUserPromptTemplate(userPromptTemplate *PromptTemplate) *Session {
	if userPromptTemplate != nil {
		s.userPromptTemplate = userPromptTemplate.Clone()
	}
	return s
}

func (s *Session) WithSystemPrompt(systemPrompt string) *Session {
	if systemPrompt != "" {
		s.systemPromptTemplate = NewPromptTemplate().WithTemplate(systemPrompt)
	}
	return s
}

func (s *Session) WithSystemPromptTemplate(systemPromptTemplate *PromptTemplate) *Session {
	if systemPromptTemplate != nil {
		s.systemPromptTemplate = systemPromptTemplate.Clone()
	}
	return s
}

func (s *Session) WithMessages(messageList ...messages.Message) *Session {
	if len(messageList) > 0 {
		s.messages = slices.Clone(messageList)
	}
	return s
}

func (s *Session) WithMiddlewares(middlewares ...any) *Session {
	if len(middlewares) > 0 {
		s.middlewareManager = NewMiddlewareManager().UseMiddlewares(middlewares...)
	}
	return s
}

func (s *Session) WithMiddlewareManager(middlewareManager *MiddlewareManager) *Session {
	if middlewareManager != nil {
		s.middlewareManager = middlewareManager.Clone()
	}
	return s
}

func (s *Session) WithParams(paramMap map[string]any) *Session {
	if len(paramMap) > 0 {
		s.params = maps.Clone(paramMap)
	}
	return s
}

func (s *Session) WithTools(toolList ...tool.Tool) *Session {
	if len(toolList) > 0 {
		s.tools = slices.Clone(toolList)
	}
	return s
}

func (s *Session) WithToolParams(toolParamMap map[string]any) *Session {
	if len(toolParamMap) > 0 {
		s.toolParams = maps.Clone(toolParamMap)
	}
	return s
}

func (s *Session) Clone() *Session {
	clonedSession, _ := NewSession(s.chatModel)
	clonedSession.
		WithChatOptions(s.chatOptions).
		WithUserPromptTemplate(s.userPromptTemplate).
		WithSystemPromptTemplate(s.systemPromptTemplate).
		WithMessages(s.messages...).
		WithMiddlewareManager(s.middlewareManager).
		WithParams(s.params).
		WithTools(s.tools...).
		WithToolParams(s.toolParams)

	return clonedSession
}

func (s *Session) ChatOptions() chat.Options {
	var resolvedChatOptions chat.Options

	if s.chatOptions != nil {
		resolvedChatOptions = s.chatOptions.Clone()
	} else {
		resolvedChatOptions = s.chatModel.DefaultOptions().Clone()
	}

	if toolOptions, ok := resolvedChatOptions.(tool.Options); ok {
		toolOptions.AddTools(s.tools)
		toolOptions.AddToolParams(s.toolParams)
	}

	return resolvedChatOptions
}

// NormalizeMessages processes and normalizes message sequences for AI conversation systems.
// This method ensures proper message structure and optimizes message sequences
// for AI model consumption.
//
// Processing Flow:
// 1. Initialize message list from template if empty
// 2. Process system messages (merge existing or render from template)
// 3. Add non-system messages (User, Assistant, Tool types)
// 4. Merge adjacent same-type messages for optimization
//
// Message Type Priority:
// - System messages: Always placed at the beginning
// - Other messages: Maintain original order (User, Assistant, Tool)
//
// Empty Message Handling:
// - If userPromptTemplate exists: render from template
// - Otherwise: create default greeting message "Hi!"
//
// Returns:
// - Processed message slice with merged adjacent same-type messages
// - Error if message rendering fails
func (s *Session) NormalizeMessages() ([]messages.Message, error) {
	workingMessages := slices.Clone(s.messages)

	// Case 1: Handle empty message list
	// Initialize conversation with user prompt template or default greeting
	if len(workingMessages) == 0 {
		if s.userPromptTemplate != nil {
			renderedUserMessage, err := s.userPromptTemplate.RenderUserMessage()
			if err != nil {
				return nil, errors.Join(err, errors.New("failed to render user prompt template"))
			}
			workingMessages = append(workingMessages, renderedUserMessage)
		} else {
			// Use friendly greeting as fallback to ensure conversation can start
			workingMessages = append(workingMessages, messages.NewUserMessage("Hi!"))
		}
	}

	// Pre-allocate capacity for performance optimization
	// Reserve space for existing messages plus potential system message
	normalizedMessages := make([]messages.Message, 0, len(workingMessages)+1)

	// Case 2: System message processing with priority-based selection
	// Strategy: Existing system messages take precedence over template-generated ones
	// Note: If neither existing system messages nor template exists, no system message is added
	mergedSystemMessage := messages.MergeSystemMessages(workingMessages)
	if mergedSystemMessage != nil {
		// Priority 1: Use merged existing system messages
		normalizedMessages = append(normalizedMessages, mergedSystemMessage)
	} else if s.systemPromptTemplate != nil {
		// Priority 2: Generate system message from template when no existing ones found
		renderedSystemMessage, err := s.systemPromptTemplate.RenderSystemMessage()
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to render system prompt template"))
		}
		normalizedMessages = append(normalizedMessages, renderedSystemMessage)
	}

	// Case 3: Add non-system messages while preserving order
	// Filter out system messages to prevent duplication since they're already processed above
	// Only include User, Assistant, and Tool messages in their original sequence
	nonSystemMessages := messages.FilterByTypes(workingMessages, messages.User, messages.Assistant, messages.Tool)
	normalizedMessages = append(normalizedMessages, nonSystemMessages...)

	// Case 4: Final optimization - merge adjacent messages of the same type
	// This step combines consecutive messages of identical types to reduce redundancy
	// and optimize the message sequence for better AI model consumption
	// Example: [User1, User2, System, User3, Tool1, Tool2] → [MergedUser(1+2), System, User3, MergedTool(1+2)]
	return messages.MergeAdjacentSameTypeMessages(normalizedMessages), nil
}
