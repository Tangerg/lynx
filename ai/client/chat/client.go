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

type Client struct {
	defaultRequest *Request
}

func NewClient(request *Request) (*Client, error) {
	if request == nil {
		return nil, errors.New("request is required")
	}
	return &Client{
		defaultRequest: request,
	}, nil
}

func (c *Client) Chat() *Request {
	return c.defaultRequest.Clone()
}

func (c *Client) ChatText(text string) *Request {
	userMessage := messages.NewUserMessage(text, nil)
	chatRequest := request.
		NewChatRequestBuilder().
		WithMessages(userMessage).
		WithOptions(c.defaultRequest.chatOptions.Clone()).
		MustBuild()
	return c.ChatRequest(chatRequest)
}

func (c *Client) ChatRequest(request *request.ChatRequest) *Request {
	if request == nil {
		panic("request is required")
	}

	req := c.defaultRequest.Clone()

	if request.Options() != nil {
		req.SetChatOptions(request.Options())
	}
	if len(request.Instructions()) > 0 {
		req.SetMessages(request.Instructions()...)
	}

	return req
}

func (c *Client) Fork() *Builder {
	b := NewBuilder().
		WithChatModel(c.defaultRequest.chatModel).
		WithChatOptions(c.defaultRequest.chatOptions).
		WithUserPromptTemplate(c.defaultRequest.userPromptTemplate).
		WithSystemPromptTemplate(c.defaultRequest.systemPromptTemplate).
		WithMessages(c.defaultRequest.messages...).
		WithMiddlewares(c.defaultRequest.middlewares).
		WithMiddlewareParams(c.defaultRequest.middlewareParams).
		WithTools(c.defaultRequest.tools...).
		WithToolParams(c.defaultRequest.toolParams)
	return b
}

type Builder struct {
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

func NewBuilder() *Builder {
	return &Builder{
		userPromptTemplate:   NewUserPromptTemplate().SetTemplate("Hi!"),
		systemPromptTemplate: NewSystemPromptTemplate(),
		messages:             make([]messages.Message, 0),
		middlewares:          NewMiddlewares(),
		middlewareParams:     make(map[string]any),
		tools:                make([]tool.Tool, 0),
		toolParams:           make(map[string]any),
	}
}

func (b *Builder) WithChatModel(chatModel model.ChatModel) *Builder {
	if chatModel != nil {
		b.chatModel = chatModel
	}
	return b
}

func (b *Builder) WithChatOptions(chatOptions request.ChatOptions) *Builder {
	if chatOptions != nil {
		b.chatOptions = chatOptions.Clone()
	}
	return b
}

func (b *Builder) WithUserPrompt(userPrompt string) *Builder {
	if userPrompt != "" {
		b.userPromptTemplate = NewUserPromptTemplate().SetTemplate(userPrompt)
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
		b.systemPromptTemplate = NewSystemPromptTemplate().SetTemplate(systemPrompt)
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
	newRequest, err := NewRequest(b.chatModel)
	if err != nil {
		return nil, err
	}
	newRequest.
		SetChatOptions(b.chatOptions).
		SetUserPromptTemplate(b.userPromptTemplate).
		SetSystemPromptTemplate(b.systemPromptTemplate).
		SetMessages(b.messages...).
		SetMiddlewares(b.middlewares).
		SetMiddlewareParams(b.middlewareParams).
		SetTools(b.tools...).
		SetToolParams(b.toolParams)

	return NewClient(newRequest)
}

func (b *Builder) MustBuild() *Client {
	build, err := b.Build()
	if err != nil {
		panic(err)
	}
	return build
}
