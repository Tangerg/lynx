package runtime

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

var errNilConversationContext = errors.New("runtime: BindConversation returned a nil context")

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
	guardrails := p.effectiveGuardrails()
	callCapacity, streamCapacity := 1, 1
	if guardrails != nil {
		callCapacity += len(guardrails.CallMiddlewares)
		streamCapacity += len(guardrails.StreamMiddlewares)
	}
	callMiddleware := make([]chat.CallMiddleware, 0, callCapacity)
	streamMiddleware := make([]chat.StreamMiddleware, 0, streamCapacity)
	conversationID := p.conversationID()
	if conversationID != "" && guardrails != nil && guardrails.BindConversation != nil {
		callMiddleware = append(callMiddleware, bindCallConversation(conversationID, guardrails.BindConversation))
		streamMiddleware = append(streamMiddleware, bindStreamConversation(conversationID, guardrails.BindConversation))
	}
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

func bindCallConversation(id string, bind func(context.Context, string) context.Context) chat.CallMiddleware {
	return func(next chat.Model) chat.Model {
		return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
			ctx = bind(ctx, id)
			if ctx == nil {
				return nil, errNilConversationContext
			}
			return next.Call(ctx, request)
		})
	}
}

func bindStreamConversation(id string, bind func(context.Context, string) context.Context) chat.StreamMiddleware {
	return func(next chat.Streamer) chat.Streamer {
		return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
			ctx = bind(ctx, id)
			if ctx == nil {
				return func(yield func(*chat.Response, error) bool) {
					yield(nil, errNilConversationContext)
				}
			}
			return next.Stream(ctx, request)
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
