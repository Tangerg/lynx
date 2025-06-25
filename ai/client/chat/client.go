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
	defaultOptions *Options
}

func NewClient(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("options is required")
	}
	return &Client{
		defaultOptions: options,
	}, nil
}

func (c *Client) Chat() *Options {
	return c.defaultOptions.Clone()
}

func (c *Client) ChatText(text string) *Options {
	userMessage := messages.NewUserMessage(text)
	chatRequest, _ := chat.NewRequest([]messages.Message{userMessage}, c.defaultOptions.chatOptions.Clone())

	return c.ChatRequest(chatRequest)
}

func (c *Client) ChatRequest(chatRequest *chat.Request) *Options {
	options := c.defaultOptions.Clone()

	if chatRequest.Options() != nil {
		options.WithChatOptions(chatRequest.Options())
	}
	if len(chatRequest.Instructions()) > 0 {
		options.WithMessages(chatRequest.Instructions()...)
	}

	return options
}

func (c *Client) Fork() *Builder {
	b := NewBuilder().
		WithChatModel(c.defaultOptions.chatModel).
		WithChatOptions(c.defaultOptions.chatOptions).
		WithUserPromptTemplate(c.defaultOptions.userPromptTemplate).
		WithSystemPromptTemplate(c.defaultOptions.systemPromptTemplate).
		WithMessages(c.defaultOptions.messages...).
		WithMiddlewares(c.defaultOptions.middlewares).
		WithMiddlewareParams(c.defaultOptions.middlewareParams).
		WithTools(c.defaultOptions.tools...).
		WithToolParams(c.defaultOptions.toolParams)
	return b
}

type Builder struct {
	chatModel            chat.Model
	chatOptions          chat.Options
	userPromptTemplate   *UserPromptTemplate
	systemPromptTemplate *SystemPromptTemplate
	messages             []messages.Message
	middlewares          *Middlewares
	middlewareParams     map[string]any
	tools                []tool.Tool
	toolParams           map[string]any
}

func NewBuilder() *Builder {
	return &Builder{
		messages:         make([]messages.Message, 0),
		middlewares:      NewMiddlewares(),
		middlewareParams: make(map[string]any),
		tools:            make([]tool.Tool, 0),
		toolParams:       make(map[string]any),
	}
}

func (b *Builder) WithChatModel(chatModel chat.Model) *Builder {
	if chatModel != nil {
		b.chatModel = chatModel
	}
	return b
}

func (b *Builder) WithChatOptions(chatOptions chat.Options) *Builder {
	if chatOptions != nil {
		b.chatOptions = chatOptions.Clone()
	}
	return b
}

func (b *Builder) WithUserPrompt(userPrompt string) *Builder {
	if userPrompt != "" {
		b.userPromptTemplate = NewUserPromptTemplate().WithTemplate(userPrompt)
	}
	return b
}

func (b *Builder) WithUserPromptTemplate(userPromptTemplate *UserPromptTemplate) *Builder {
	if userPromptTemplate != nil {
		b.userPromptTemplate = userPromptTemplate.Clone()
	}
	return b
}

func (b *Builder) WithSystemPrompt(systemPrompt string) *Builder {
	if systemPrompt != "" {
		b.systemPromptTemplate = NewSystemPromptTemplate().WithTemplate(systemPrompt)
	}
	return b
}

func (b *Builder) WithSystemPromptTemplate(systemPrompt *SystemPromptTemplate) *Builder {
	if systemPrompt != nil {
		b.systemPromptTemplate = systemPrompt.Clone()
	}
	return b
}

func (b *Builder) WithMessages(messages ...messages.Message) *Builder {
	if len(messages) > 0 {
		b.messages = slices.Clone(messages)
	}
	return b
}

func (b *Builder) WithMiddlewares(middlewares *Middlewares) *Builder {
	if middlewares != nil {
		b.middlewares = middlewares.Clone()
	}
	return b
}

func (b *Builder) WithMiddlewareParams(middlewareParams map[string]any) *Builder {
	if len(middlewareParams) > 0 {
		b.middlewareParams = maps.Clone(middlewareParams)
	}
	return b
}

func (b *Builder) WithTools(tools ...tool.Tool) *Builder {
	if len(tools) > 0 {
		b.tools = slices.Clone(tools)
	}
	return b
}

func (b *Builder) WithToolParams(toolParams map[string]any) *Builder {
	if len(toolParams) > 0 {
		b.toolParams = maps.Clone(toolParams)
	}
	return b
}

func (b *Builder) Build() (*Client, error) {
	newOptions, err := NewOptions(b.chatModel)
	if err != nil {
		return nil, err
	}
	newOptions.
		WithChatOptions(b.chatOptions).
		WithUserPromptTemplate(b.userPromptTemplate).
		WithSystemPromptTemplate(b.systemPromptTemplate).
		WithMessages(b.messages...).
		WithMiddlewares(b.middlewares).
		WithMiddlewareParams(b.middlewareParams).
		WithTools(b.tools...).
		WithToolParams(b.toolParams)

	return NewClient(newOptions)
}

func (b *Builder) MustBuild() *Client {
	build, err := b.Build()
	if err != nil {
		panic(err)
	}
	return build
}
