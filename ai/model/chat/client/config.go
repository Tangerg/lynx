package client

import (
	"errors"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/chat/tool"
)

type Config struct {
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

func NewConfig(chatModel chat.Model) (*Config, error) {
	if chatModel == nil {
		return nil, errors.New("chatModel is required")
	}

	return &Config{
		chatModel:         chatModel,
		middlewareManager: NewMiddlewareManager(),
		params:            make(map[string]any),
		tools:             make([]tool.Tool, 0),
		toolParams:        make(map[string]any),
	}, nil
}

func (c *Config) Call() *Caller {
	chatCaller, _ := NewCaller(c)
	return chatCaller
}

func (c *Config) Stream() *Streamer {
	chatStreamer, _ := NewStreamer(c)
	return chatStreamer
}

func (c *Config) WithChatOptions(chatOptions chat.Options) *Config {
	if chatOptions != nil {
		c.chatOptions = chatOptions
	}
	return c
}

func (c *Config) WithUserPrompt(prompt string) *Config {
	if prompt != "" {
		c.userPromptTemplate = NewPromptTemplate().WithTemplate(prompt)
	}
	return c
}

func (c *Config) WithUserPromptTemplate(userPromptTemplate *PromptTemplate) *Config {
	if userPromptTemplate != nil {
		c.userPromptTemplate = userPromptTemplate
	}
	return c
}

func (c *Config) WithSystemPrompt(prompt string) *Config {
	if prompt != "" {
		c.systemPromptTemplate = NewPromptTemplate().WithTemplate(prompt)
	}
	return c
}

func (c *Config) WithSystemPromptTemplate(systemPromptTemplate *PromptTemplate) *Config {
	if systemPromptTemplate != nil {
		c.systemPromptTemplate = systemPromptTemplate
	}
	return c
}

func (c *Config) WithMessages(messages ...messages.Message) *Config {
	if len(messages) > 0 {
		c.messages = messages
	}
	return c
}

func (c *Config) WithMiddlewares(middlewares ...any) *Config {
	if len(middlewares) > 0 {
		c.middlewareManager = NewMiddlewareManager().UseMiddlewares(middlewares...)
	}
	return c
}

func (c *Config) WithMiddlewareManager(middlewareManager *MiddlewareManager) *Config {
	if middlewareManager != nil {
		c.middlewareManager = middlewareManager
	}
	return c
}

func (c *Config) WithParams(params map[string]any) *Config {
	if params != nil {
		c.params = params
	}
	return c
}

func (c *Config) WithTools(tools ...tool.Tool) *Config {
	if len(tools) > 0 {
		c.tools = tools
	}
	return c
}

func (c *Config) WithToolParams(toolParams map[string]any) *Config {
	if toolParams != nil {
		c.toolParams = toolParams
	}
	return c
}

func (c *Config) Clone() *Config {
	clonedConfig, _ := NewConfig(c.chatModel)

	clonedConfig.
		WithChatOptions(c.chatOptions).
		WithUserPromptTemplate(c.userPromptTemplate).
		WithSystemPromptTemplate(c.systemPromptTemplate).
		WithMessages(c.messages...).
		WithMiddlewareManager(c.middlewareManager).
		WithParams(c.params).
		WithTools(c.tools...).
		WithToolParams(c.toolParams)

	return clonedConfig
}

func (c *Config) getChatOptions() chat.Options {
	var mergedChatOptions chat.Options

	if c.chatOptions != nil {
		mergedChatOptions = c.chatOptions.Clone()
	} else {
		mergedChatOptions = c.chatModel.DefaultOptions().Clone()
	}

	toolOptions, isToolCapable := mergedChatOptions.(tool.Options)
	if isToolCapable {
		toolOptions.AddTools(c.tools)
		toolOptions.AddToolParams(c.toolParams)
	}

	return mergedChatOptions
}

// getMessages processes and normalizes message sequences for AI conversation systems.
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
func (c *Config) getMessages() ([]messages.Message, error) {
	workingMessageList := slices.Clone(c.messages)

	// Case 1: Handle empty message list
	// Initialize conversation with user prompt template or default greeting
	if len(workingMessageList) == 0 {
		if c.userPromptTemplate != nil {
			renderedUserMessage, renderErr := c.userPromptTemplate.RenderUserMessage()
			if renderErr != nil {
				return nil, errors.Join(renderErr, errors.New("failed to render user prompt template"))
			}
			workingMessageList = append(workingMessageList, renderedUserMessage)
		} else {
			// Use friendly greeting as fallback to ensure conversation can start
			defaultUserMessage := messages.NewUserMessage("Hi!")
			workingMessageList = append(workingMessageList, defaultUserMessage)
		}
	}

	// Pre-allocate capacity for performance optimization
	// Reserve space for existing messages plus potential system message
	normalizedMessageList := make([]messages.Message, 0, len(workingMessageList)+1)

	// Case 2: System message processing with priority-based selection
	// Strategy: Existing system messages take precedence over template-generated ones
	// Note: If neither existing system messages nor template exists, no system message is added
	mergedSystemMessage := messages.MergeSystemMessages(workingMessageList)
	if mergedSystemMessage != nil {
		// Priority 1: Use merged existing system messages
		normalizedMessageList = append(normalizedMessageList, mergedSystemMessage)
	} else if c.systemPromptTemplate != nil {
		// Priority 2: Generate system message from template when no existing ones found
		renderedSystemMessage, renderErr := c.systemPromptTemplate.RenderSystemMessage()
		if renderErr != nil {
			return nil, errors.Join(renderErr, errors.New("failed to render system prompt template"))
		}
		normalizedMessageList = append(normalizedMessageList, renderedSystemMessage)
	}

	// Case 3: Add non-system messages while preserving order
	// Filter out system messages to prevent duplication since they're already processed above
	// Only include User, Assistant, and Tool messages in their original sequence
	filteredNonSystemMessages := messages.FilterByTypes(workingMessageList, messages.User, messages.Assistant, messages.Tool)
	normalizedMessageList = append(normalizedMessageList, filteredNonSystemMessages...)

	// Case 4: Final optimization - merge adjacent messages of the same type
	// This step combines consecutive messages of identical types to reduce redundancy
	// and optimize the message sequence for better AI model consumption
	// Example: [User1, User2, System, User3, Tool1, Tool2] → [MergedUser(1+2), System, User3, MergedTool(1+2)]
	finalOptimizedMessageList := messages.MergeAdjacentSameTypeMessages(normalizedMessageList)

	return finalOptimizedMessageList, nil
}

func (c *Config) getMiddlewareManager() *MiddlewareManager {
	if c.middlewareManager == nil {
		c.middlewareManager = NewMiddlewareManager()
	}
	return c.middlewareManager
}

func (c *Config) toChatRequest() (*chat.Request, error) {
	normalizedMessageList, err := c.getMessages()
	if err != nil {
		return nil, err
	}

	chatRequest, err := chat.NewRequest(normalizedMessageList, c.getChatOptions())
	if err != nil {
		return nil, err
	}

	chatRequest.SetParams(c.params)

	return chatRequest, nil
}
