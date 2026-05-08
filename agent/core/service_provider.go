package core

import "sync"

// ServiceProvider is the open-ended service registry exposed to
// actions via [ProcessContext.Services]. Users register whatever
// services their actions need — LLM clients, RAG engines, custom
// domain services — by string key; actions look them up via
// [ServiceOf] for type safety, or via [ServiceProvider.Get] for the
// raw [any].
//
// The framework deliberately does not pre-define typed slots (Chat /
// RAG / VectorStore / …). Different deployments use different
// libraries; a fixed shape would tie the framework to one ecosystem.
//
// All methods are safe for concurrent use.
type ServiceProvider struct {
	mu       sync.RWMutex
	services map[string]any
}

// NewServiceProvider returns an empty registry.
func NewServiceProvider() *ServiceProvider {
	return &ServiceProvider{services: map[string]any{}}
}

// Get returns the value registered at key plus an ok flag.
func (p *ServiceProvider) Get(key string) (any, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	v, ok := p.services[key]
	return v, ok
}

// Set registers (or replaces) the value at key.
func (p *ServiceProvider) Set(key string, value any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.services == nil {
		p.services = map[string]any{}
	}
	p.services[key] = value
}

// ServiceOf is the typed lookup helper: the value registered at key
// cast to T, plus an ok flag. ok is false when the key is missing or
// the value isn't assignable to T.
func ServiceOf[T any](p *ServiceProvider, key string) (T, bool) {
	var zero T
	v, ok := p.Get(key)
	if !ok {
		return zero, false
	}
	typed, ok := v.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}
