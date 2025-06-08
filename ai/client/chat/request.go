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

type Request struct {
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

func NewRequest(chatModel model.ChatModel) (*Request, error) {
	if chatModel == nil {
		return nil, errors.New("chatModel is required")
	}
	return &Request{
		chatModel:            chatModel,
		chatOptions:          chatModel.DefaultOptions().Clone(),
		userPromptTemplate:   NewUserPromptTemplate().SetTemplate("Hi!"),
		systemPromptTemplate: NewSystemPromptTemplate(),
		messages:             make([]messages.Message, 0),
		middlewares:          NewMiddlewares(),
		middlewareParams:     make(map[string]any),
		tools:                make([]tool.Tool, 0),
		toolParams:           make(map[string]any),
	}, nil
}

func (r *Request) Call() *Call {
	callRequest, _ := NewCall(r, r.middlewares)
	return callRequest
}

func (r *Request) Stream() *Stream {
	streamRequest, _ := NewStream(r, r.middlewares)
	return streamRequest
}

func (r *Request) SetChatOptions(options request.ChatOptions) *Request {
	if options != nil {
		r.chatOptions = options.Clone()
	}
	return r
}

func (r *Request) SetUserPromptTemplate(userPrompt *UserPromptTemplate) *Request {
	if userPrompt != nil {
		r.userPromptTemplate = userPrompt.Clone()
	}
	return r
}

func (r *Request) SetSystemPromptTemplate(systemPrompt *SystemPromptTemplate) *Request {
	if systemPrompt != nil {
		r.systemPromptTemplate = systemPrompt.Clone()
	}
	return r
}

func (r *Request) SetMessages(messages ...messages.Message) *Request {
	if len(messages) > 0 {
		r.messages = slices.Clone(messages)
	}
	return r
}

func (r *Request) SetMiddlewares(middlewares *Middlewares) *Request {
	if middlewares != nil {
		r.middlewares = middlewares.Clone()
	}
	return r
}

func (r *Request) AddMiddlewares(middlewares ...any) *Request {
	if len(middlewares) > 0 {
		r.middlewares.Add(middlewares...)
	}
	return r
}

func (r *Request) SetMiddlewareParams(params map[string]any) *Request {
	if len(params) > 0 {
		r.middlewareParams = maps.Clone(params)
	}
	return r
}

func (r *Request) SetTools(tools ...tool.Tool) *Request {
	if len(tools) > 0 {
		r.tools = slices.Clone(tools)
	}
	return r
}

func (r *Request) SetToolParams(params map[string]any) *Request {
	if len(params) > 0 {
		r.toolParams = maps.Clone(params)
	}
	return r
}

func (r *Request) Clone() *Request {
	newRequest, _ := NewRequest(r.chatModel)
	newRequest.
		SetChatOptions(r.chatOptions).
		SetUserPromptTemplate(r.userPromptTemplate).
		SetSystemPromptTemplate(r.systemPromptTemplate).
		SetMessages(r.messages...).
		SetMiddlewares(r.middlewares).
		SetMiddlewareParams(r.middlewareParams).
		SetTools(r.tools...).
		SetToolParams(r.toolParams)
	return newRequest
}
