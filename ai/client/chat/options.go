package chat

import (
	"errors"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/chat/model"
	"github.com/Tangerg/lynx/ai/model/chat/request"
	"github.com/Tangerg/lynx/ai/model/tool"
)

type Options struct {
	chatModel            model.ChatModel
	chatOptions          request.ChatOptions
	userPromptTemplate   *UserPromptTemplate
	systemPromptTemplate *SystemPromptTemplate
	messages             []messages.Message
	middlewares          *Middlewares
	middlewareParams     map[string]any
	tools                []tool.Tool
	toolParams           map[string]any
}

func NewOptions(chatModel model.ChatModel) (*Options, error) {
	if chatModel == nil {
		return nil, errors.New("chatModel is required")
	}
	return &Options{
		chatModel:        chatModel,
		middlewares:      NewMiddlewares(),
		middlewareParams: make(map[string]any),
		tools:            make([]tool.Tool, 0),
		toolParams:       make(map[string]any),
	}, nil
}

func (o *Options) Call() *Caller {
	caller, _ := NewCaller(o)
	return caller
}

func (o *Options) Stream() *Streamer {
	streamer, _ := NewStreamer(o)
	return streamer
}

func (o *Options) WithChatOptions(chatOptions request.ChatOptions) *Options {
	if chatOptions != nil {
		o.chatOptions = chatOptions.Clone()
	}
	return o
}

func (o *Options) WithUserPrompt(userPrompt string) *Options {
	if userPrompt != "" {
		o.userPromptTemplate = NewUserPromptTemplate().WithTemplate(userPrompt)
	}
	return o
}

func (o *Options) WithUserPromptTemplate(userPrompt *UserPromptTemplate) *Options {
	if userPrompt != nil {
		o.userPromptTemplate = userPrompt.Clone()
	}
	return o
}

func (o *Options) WithSystemPrompt(systemPrompt string) *Options {
	if systemPrompt != "" {
		o.systemPromptTemplate = NewSystemPromptTemplate().WithTemplate(systemPrompt)
	}
	return o
}

func (o *Options) WithSystemPromptTemplate(systemPrompt *SystemPromptTemplate) *Options {
	if systemPrompt != nil {
		o.systemPromptTemplate = systemPrompt.Clone()
	}
	return o
}

func (o *Options) WithMessages(messages ...messages.Message) *Options {
	if len(messages) > 0 {
		o.messages = slices.Clone(messages)
	}
	return o
}

func (o *Options) WithMiddlewares(middlewares *Middlewares) *Options {
	if middlewares != nil {
		o.middlewares = middlewares.Clone()
	}
	return o
}

func (o *Options) WithMiddlewareParams(params map[string]any) *Options {
	if len(params) > 0 {
		o.middlewareParams = maps.Clone(params)
	}
	return o
}

func (o *Options) WithTools(tools ...tool.Tool) *Options {
	if len(tools) > 0 {
		o.tools = slices.Clone(tools)
	}
	return o
}

func (o *Options) WithToolParams(params map[string]any) *Options {
	if len(params) > 0 {
		o.toolParams = maps.Clone(params)
	}
	return o
}

func (o *Options) Clone() *Options {
	newOptions, _ := NewOptions(o.chatModel)
	newOptions.
		WithChatOptions(o.chatOptions).
		WithUserPromptTemplate(o.userPromptTemplate).
		WithSystemPromptTemplate(o.systemPromptTemplate).
		WithMessages(o.messages...).
		WithMiddlewares(o.middlewares).
		WithMiddlewareParams(o.middlewareParams).
		WithTools(o.tools...).
		WithToolParams(o.toolParams)
	return newOptions
}

func (o *Options) ChatOptions() request.ChatOptions {
	var chatOptions request.ChatOptions

	if o.chatOptions != nil {
		chatOptions = o.chatOptions.Clone()
	} else {
		chatOptions = o.chatModel.DefaultOptions().Clone()
	}

	return chatOptions
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
func (o *Options) NormalizeMessages() ([]messages.Message, error) {
	msgs := slices.Clone(o.messages)

	// Case 1: Handle empty message list
	// Initialize conversation with user prompt template or default greeting
	if len(msgs) == 0 {
		if o.userPromptTemplate != nil {
			userMessage, err := o.userPromptTemplate.RenderMessage()
			if err != nil {
				return nil, errors.Join(err, errors.New("failed to render user prompt template"))
			}
			msgs = append(msgs, userMessage)
		} else {
			// Use friendly greeting as fallback to ensure conversation can start
			msgs = append(msgs, messages.NewUserMessage("Hi!", nil))
		}
	}

	// Pre-allocate capacity for performance optimization
	// Reserve space for existing messages plus potential system message
	processedMsgs := make([]messages.Message, 0, len(msgs)+1)

	// Case 2: System message processing with priority-based selection
	// Strategy: Existing system messages take precedence over template-generated ones
	// Note: If neither existing system messages nor template exists, no system message is added
	systemMsg := messages.MergeSystemMessages(msgs)
	if systemMsg != nil {
		// Priority 1: Use merged existing system messages
		processedMsgs = append(processedMsgs, systemMsg)
	} else if o.systemPromptTemplate != nil {
		// Priority 2: Generate system message from template when no existing ones found
		renderedSystemMsg, err := o.systemPromptTemplate.RenderMessage()
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to render system prompt template"))
		}
		processedMsgs = append(processedMsgs, renderedSystemMsg)
	}

	// Case 3: Add non-system messages while preserving order
	// Filter out system messages to prevent duplication since they're already processed above
	// Only include User, Assistant, and Tool messages in their original sequence
	otherTypeMsg := messages.FilterByTypes(msgs, messages.User, messages.Assistant, messages.Tool)
	processedMsgs = append(processedMsgs, otherTypeMsg...)

	// Case 4: Final optimization - merge adjacent messages of the same type
	// This step combines consecutive messages of identical types to reduce redundancy
	// and optimize the message sequence for better AI model consumption
	// Example: [User1, User2, System, User3, Tool1, Tool2] → [MergedUser(1+2), System, User3, MergedTool(1+2)]
	return messages.MergeAdjacentSameTypeMessages(processedMsgs), nil
}
