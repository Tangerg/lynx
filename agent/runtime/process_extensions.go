package runtime

import (
	"reflect"

	"github.com/Tangerg/lynx/agent/core"
)

// platformServices returns the platform's open service registry, or a
// fresh empty one when there's no platform attached (test fixtures).
func (p *AgentProcess) platformServices() *core.ServiceProvider {
	if p.platform == nil {
		return core.NewServiceProvider()
	}
	return p.platform.services
}

// platformChatClient returns the platform's shared [core.ChatClient], or
// nil when the platform was constructed without one (or when there's
// no platform attached — test fixtures). Action code reaches this via
// ProcessContext.Chat / ChatWithActionTools.
func (p *AgentProcess) platformChatClient() core.ChatClient {
	if p.platform == nil {
		return nil
	}
	return p.platform.chatClient
}

// effectiveChatClient returns the chat client this process's actions use:
// the first non-nil client from a registered [core.ChatClientProvider]
// (process scope first, so a per-process override beats a platform default),
// else the platform's shared client. This is what lets one Platform serve
// turns against different models without a Platform per model. Mirrors the
// resolver-first ordering used for tool group resolution.
func (p *AgentProcess) effectiveChatClient() core.ChatClient {
	providers := collectExtensions[core.ChatClientProvider](p.combinedExtensionsResolverFirst())
	for _, prov := range providers {
		if c := normalizeChatClient(prov.ChatClientFor(p)); c != nil {
			return c
		}
	}
	return p.platformChatClient()
}

// normalizeChatClient collapses typed nil implementations stored in the
// interface so provider overrides can still fall back to the platform default.
func normalizeChatClient(client core.ChatClient) core.ChatClient {
	if client == nil {
		return nil
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil
		}
	}
	return client
}

// platformGuardrails returns the platform-level chat guardrails, or
// nil when none are configured (or no platform attached). Threaded
// into ProcessContext so [ProcessContext.Chat] can pre-install the
// global middlewares on every request.
func (p *AgentProcess) platformGuardrails() *core.Guardrails {
	if p.platform == nil {
		return nil
	}
	return p.platform.guardrails
}

// effectiveGuardrails returns the process-scoped guardrails when set
// ([core.ProcessOptions.Guardrails]), falling back to the platform
// default. Called once per tick by [buildProcessContext].
func (p *AgentProcess) effectiveGuardrails() *core.Guardrails {
	if p.options != nil && p.options.Guardrails != nil {
		return p.options.Guardrails
	}
	return p.platformGuardrails()
}

// platformExtensions exposes the platform-scoped extension list.
func (p *AgentProcess) platformExtensions() []core.Extension {
	if p.platform == nil {
		return nil
	}
	return p.platform.extensions.list
}

// processExtensions exposes the per-process extension list (from
// [core.ProcessOptions.Extensions]).
func (p *AgentProcess) processExtensions() []core.Extension {
	if p.options == nil {
		return nil
	}
	return p.options.Extensions
}

// combinedExtensions returns platform extensions followed by process
// extensions — the natural ordering for onion / wrap chains where
// platform sits outermost (registered earliest) and process sits
// innermost (registered last). Goal-approver dispatch reads this list.
func (p *AgentProcess) combinedExtensions() []core.Extension {
	return mergeExtensions(p.platformExtensions(), p.processExtensions())
}

// combinedExtensionsResolverFirst returns process extensions BEFORE
// platform extensions — the order used for first-hit resolvers so a
// process-scope override is consulted first.
func (p *AgentProcess) combinedExtensionsResolverFirst() []core.Extension {
	return mergeExtensions(p.processExtensions(), p.platformExtensions())
}

// mergeExtensions concatenates first then second, returning the input
// directly (no allocation) when either side is empty.
func mergeExtensions(first, second []core.Extension) []core.Extension {
	if len(second) == 0 {
		return first
	}
	if len(first) == 0 {
		return second
	}
	out := make([]core.Extension, 0, len(first)+len(second))
	out = append(out, first...)
	out = append(out, second...)
	return out
}
