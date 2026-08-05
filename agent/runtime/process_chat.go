package runtime

import (
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/chatclient"
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
		candidate, err := chatFromProvider(provider.value, p, provider.name)
		if err != nil {
			return core.ChatCapability{}, err
		}
		if nilvalue.Is(candidate.Model) {
			if !nilvalue.Is(candidate.Streamer) {
				return core.ChatCapability{}, fmt.Errorf("runtime: ChatProvider %q returned a Streamer without a Model", provider.name)
			}
			continue
		}
		capability = candidate
		break
	}
	if nilvalue.Is(capability.Model) {
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
	if nilvalue.Is(capability.Model) {
		return core.ChatCapability{}, nil
	}
	middleware := p.effectiveChatMiddleware()
	callCapacity, streamCapacity := 0, 0
	if middleware != nil {
		callCapacity += len(middleware.CallMiddlewares)
		streamCapacity += len(middleware.StreamMiddlewares)
	}
	callMiddleware := make([]chat.CallMiddleware, 0, callCapacity)
	streamMiddleware := make([]chat.StreamMiddleware, 0, streamCapacity)
	if !middleware.Empty() {
		callMiddleware = append(callMiddleware, middleware.CallMiddlewares...)
		streamMiddleware = append(streamMiddleware, middleware.StreamMiddlewares...)
	}
	config := chatclient.Config{CallMiddleware: callMiddleware}
	if !nilvalue.Is(capability.Streamer) {
		config.Streamer = capability.Streamer
		config.StreamMiddleware = streamMiddleware
	}
	client, err := chatclient.New(capability.Model, config)
	if err != nil {
		return core.ChatCapability{}, err
	}
	result := core.ChatCapability{Model: client}
	if !nilvalue.Is(capability.Streamer) {
		result.Streamer = client
	}
	return result, nil
}

func (p *Process) effectiveChatMiddleware() *core.ChatMiddleware {
	var process, engine *core.ChatMiddleware
	if p.options != nil {
		process = p.options.chatMiddleware
	}
	if p.engine != nil {
		engine = p.engine.chatMiddleware
	}
	if process == nil {
		return engine
	}
	if engine == nil {
		return process
	}
	return &core.ChatMiddleware{
		CallMiddlewares: append(
			slices.Clone(process.CallMiddlewares),
			engine.CallMiddlewares...,
		),
		StreamMiddlewares: append(
			slices.Clone(process.StreamMiddlewares),
			engine.StreamMiddlewares...,
		),
	}
}

func (p *Process) effectiveMaxModelCalls() int {
	if p.options != nil && p.options.maxModelCalls != 0 {
		return p.options.maxModelCalls
	}
	if p.engine != nil {
		return p.engine.maxModelCalls
	}
	return 0
}
