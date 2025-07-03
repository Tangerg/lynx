package chat

import (
	"errors"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/tool"
)

type Client struct {
	defaultSession *Session
}

func NewClient(session *Session) (*Client, error) {
	if session == nil {
		return nil, errors.New("session is required")
	}

	return &Client{
		defaultSession: session,
	}, nil
}

func (c *Client) Chat() *Session {
	return c.defaultSession.Clone()
}

func (c *Client) ChatText(text string) *Session {
	userMessage := messages.NewUserMessage(text)

	chatRequest, _ := chat.NewRequest([]messages.Message{userMessage}, c.defaultSession.chatOptions)

	return c.ChatRequest(chatRequest)
}

func (c *Client) ChatRequest(chatRequest *chat.Request) *Session {
	clonedSession := c.defaultSession.Clone()

	if chatRequest.Options() != nil {
		clonedSession.WithChatOptions(chatRequest.Options())
	}

	if len(chatRequest.Instructions()) > 0 {
		clonedSession.WithMessages(chatRequest.Instructions()...)
	}

	return clonedSession
}

func (c *Client) Fork() *ClientBuilder {
	builder := NewClientBuilder().
		WithChatModel(c.defaultSession.chatModel).
		WithChatOptions(c.defaultSession.chatOptions).
		WithUserPromptTemplate(c.defaultSession.userPromptTemplate).
		WithSystemPromptTemplate(c.defaultSession.systemPromptTemplate).
		WithMessages(c.defaultSession.messages...).
		WithMiddlewareManager(c.defaultSession.middlewareManager).
		WithParams(c.defaultSession.params).
		WithTools(c.defaultSession.tools...).
		WithToolParams(c.defaultSession.toolParams)

	return builder
}

type ClientBuilder struct {
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

func NewClientBuilder() *ClientBuilder {
	return &ClientBuilder{
		messages:          make([]messages.Message, 0),
		middlewareManager: NewMiddlewareManager(),
		params:            make(map[string]any),
		tools:             make([]tool.Tool, 0),
		toolParams:        make(map[string]any),
	}
}

func (b *ClientBuilder) WithChatModel(chatModel chat.Model) *ClientBuilder {
	if chatModel != nil {
		b.chatModel = chatModel
	}
	return b
}

func (b *ClientBuilder) WithChatOptions(chatOptions chat.Options) *ClientBuilder {
	if chatOptions != nil {
		b.chatOptions = chatOptions.Clone()
	}
	return b
}

func (b *ClientBuilder) WithUserPrompt(userPrompt string) *ClientBuilder {
	if userPrompt != "" {
		b.userPromptTemplate = NewPromptTemplate().WithTemplate(userPrompt)
	}
	return b
}

func (b *ClientBuilder) WithUserPromptTemplate(userPromptTemplate *PromptTemplate) *ClientBuilder {
	if userPromptTemplate != nil {
		b.userPromptTemplate = userPromptTemplate.Clone()
	}
	return b
}

func (b *ClientBuilder) WithSystemPrompt(systemPrompt string) *ClientBuilder {
	if systemPrompt != "" {
		b.systemPromptTemplate = NewPromptTemplate().WithTemplate(systemPrompt)
	}
	return b
}

func (b *ClientBuilder) WithSystemPromptTemplate(systemPromptTemplate *PromptTemplate) *ClientBuilder {
	if systemPromptTemplate != nil {
		b.systemPromptTemplate = systemPromptTemplate.Clone()
	}
	return b
}

func (b *ClientBuilder) WithMessages(messageList ...messages.Message) *ClientBuilder {
	if len(messageList) > 0 {
		b.messages = slices.Clone(messageList)
	}
	return b
}

func (b *ClientBuilder) WithMiddlewares(middlewareList ...any) *ClientBuilder {
	if len(middlewareList) > 0 {
		b.middlewareManager = NewMiddlewareManager().UseMiddlewares(middlewareList...)
	}
	return b
}

func (b *ClientBuilder) WithMiddlewareManager(middlewareManager *MiddlewareManager) *ClientBuilder {
	if middlewareManager != nil {
		b.middlewareManager = middlewareManager.Clone()
	}
	return b
}

func (b *ClientBuilder) WithParams(paramMap map[string]any) *ClientBuilder {
	if len(paramMap) > 0 {
		b.params = maps.Clone(paramMap)
	}
	return b
}

func (b *ClientBuilder) WithTools(toolList ...tool.Tool) *ClientBuilder {
	if len(toolList) > 0 {
		b.tools = slices.Clone(toolList)
	}
	return b
}

func (b *ClientBuilder) WithToolParams(toolParamMap map[string]any) *ClientBuilder {
	if len(toolParamMap) > 0 {
		b.toolParams = maps.Clone(toolParamMap)
	}
	return b
}

func (b *ClientBuilder) Build() (*Client, error) {
	builtSession, err := NewSession(b.chatModel)
	if err != nil {
		return nil, err
	}

	builtSession.
		WithChatOptions(b.chatOptions).
		WithUserPromptTemplate(b.userPromptTemplate).
		WithSystemPromptTemplate(b.systemPromptTemplate).
		WithMessages(b.messages...).
		WithMiddlewareManager(b.middlewareManager).
		WithParams(b.params).
		WithTools(b.tools...).
		WithToolParams(b.toolParams)

	return NewClient(builtSession)
}

func (b *ClientBuilder) MustBuild() *Client {
	client, err := b.Build()
	if err != nil {
		panic(err)
	}
	return client
}
