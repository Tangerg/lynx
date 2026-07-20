package runtime

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

func (p *Process) engineChat() core.ChatCapability {
	if p.engine == nil {
		return core.ChatCapability{}
	}
	return p.engine.chat
}

func (p *Process) effectiveChat() (core.ChatCapability, error) {
	providers := collectExtensions[core.ChatProvider](p.combinedExtensionsResolverFirst())
	capability := core.ChatCapability{}
	for _, provider := range providers {
		name, err := extensionName(provider)
		if err != nil {
			return core.ChatCapability{}, err
		}
		candidate, err := chatFromProvider(provider, p, name)
		if err != nil {
			return core.ChatCapability{}, err
		}
		if valueIsNil(candidate.Model) {
			if !valueIsNil(candidate.Streamer) {
				return core.ChatCapability{}, fmt.Errorf("runtime: ChatProvider %q returned a Streamer without a Model", name)
			}
			continue
		}
		capability = candidate
		break
	}
	if valueIsNil(capability.Model) {
		capability = p.engineChat()
	}
	return p.scopeChat(capability)
}

func chatFromProvider(provider core.ChatProvider, process core.ProcessView, name string) (capability core.ChatCapability, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("chat provider %q panicked", name), recovered)
		}
	}()
	return provider.Chat(process), nil
}

func (p *Process) scopeChat(capability core.ChatCapability) (core.ChatCapability, error) {
	if valueIsNil(capability.Model) {
		return core.ChatCapability{}, nil
	}
	callMiddleware := make([]chat.CallMiddleware, 0, 1)
	streamMiddleware := make([]chat.StreamMiddleware, 0, 1)
	conversationID := p.conversationID()
	if conversationID != "" {
		callMiddleware = append(callMiddleware, bindCallConversation(conversationID))
		streamMiddleware = append(streamMiddleware, bindStreamConversation(conversationID))
	}
	guardrails := p.effectiveGuardrails()
	if !guardrails.Empty() {
		callMiddleware = append(callMiddleware, guardrails.CallMiddlewares...)
		streamMiddleware = append(streamMiddleware, guardrails.StreamMiddlewares...)
	}
	options := []chatclient.Option{chatclient.WithCallMiddleware(callMiddleware...)}
	if !valueIsNil(capability.Streamer) {
		options = append(options,
			chatclient.WithStreamer(capability.Streamer),
			chatclient.WithStreamMiddleware(streamMiddleware...),
		)
	}
	client, err := chatclient.New(capability.Model, options...)
	if err != nil {
		return core.ChatCapability{}, err
	}
	result := core.ChatCapability{Model: client}
	if !valueIsNil(capability.Streamer) {
		result.Streamer = client
	}
	return result, nil
}

func bindCallConversation(id string) chat.CallMiddleware {
	return func(next chat.Model) chat.Model {
		return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
			return next.Call(chathistory.WithConversationID(ctx, id), request)
		})
	}
}

func bindStreamConversation(id string) chat.StreamMiddleware {
	return func(next chat.Streamer) chat.Streamer {
		return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
			return next.Stream(chathistory.WithConversationID(ctx, id), request)
		})
	}
}

func (p *Process) engineGuardrails() *core.ChatGuardrails {
	if p.engine == nil {
		return nil
	}
	return p.engine.guardrails
}

func (p *Process) effectiveGuardrails() *core.ChatGuardrails {
	if p.options != nil && p.options.guardrails != nil {
		return p.options.guardrails
	}
	return p.engineGuardrails()
}
