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

func (o *Options) prepareMessages() ([]messages.Message, error) {
	if len(o.messages) == 0 && o.userPromptTemplate == nil {
		return nil, errors.New("at least one message is required")
	}

	processedMessages := make([]messages.Message, 0, len(o.messages)+2)

	if o.systemPromptTemplate != nil {
		if !messages.ContainsType(o.messages, messages.System) {
			systemMessage, err := o.systemPromptTemplate.RenderMessage()
			if err != nil {
				return nil, err
			}
			processedMessages = append(processedMessages, systemMessage)
		}
	}

	processedMessages = append(processedMessages, o.messages...)

	if o.userPromptTemplate != nil {
		if !messages.IsLastOfType(o.messages, messages.User) {
			userMessage, err := o.userPromptTemplate.RenderMessage()
			if err != nil {
				return nil, err
			}
			processedMessages = append(processedMessages, userMessage)
		}
	}

	return processedMessages, nil
}

func (o *Options) prepareChatOptions() request.ChatOptions {
	var chatOptions request.ChatOptions
	if o.chatOptions != nil {
		chatOptions = o.chatOptions.Clone()
	} else {
		chatOptions = o.chatModel.DefaultOptions().Clone()
	}

	return chatOptions
}
