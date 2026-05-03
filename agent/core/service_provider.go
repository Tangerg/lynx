package core

import "sync"

// ServiceProvider is the open-ended service registry exposed to actions
// via [ProcessContext.Services]. Users register whatever services their
// actions need — LLM clients, RAG engines, custom domain services — by
// string key; actions look them up via [ServiceOf] for type-safety, or
// via [ServiceProvider.Get] for the raw [any].
//
// The framework deliberately does not pre-define typed slots (Chat / RAG
// / VectorStore / …). Different deployments use different libraries; a
// fixed shape would either tie the framework to one ecosystem or force
// adapter wrappers. A generic key→value map keeps the framework agnostic
// and lets users register arbitrary types.
//
// All methods are safe for concurrent use.
type ServiceProvider struct {
	mu       sync.RWMutex
	services map[string]any
}

// NewServiceProvider returns an empty registry ready to receive Set
// calls.
func NewServiceProvider() *ServiceProvider {
	return &ServiceProvider{services: map[string]any{}}
}

// Get returns the value registered at key plus an ok flag. Calling Get
// on a nil receiver yields (nil, false) so action code can probe
// optimistically without nil-checking the provider.
func (p *ServiceProvider) Get(key string) (any, bool) {
	if p == nil {
		return nil, false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	v, ok := p.services[key]
	return v, ok
}

// Set registers (or replaces) the value at key. A nil receiver is
// silently ignored — same rationale as [Get].
func (p *ServiceProvider) Set(key string, value any) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.services == nil {
		p.services = map[string]any{}
	}
	p.services[key] = value
}

// Delete removes the entry at key, if any. A nil receiver is a no-op.
func (p *ServiceProvider) Delete(key string) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.services, key)
}

// Has reports whether key is registered.
func (p *ServiceProvider) Has(key string) bool {
	_, ok := p.Get(key)
	return ok
}

// Keys returns a snapshot of the currently registered keys, in
// unspecified order. Useful for debug listings; not a contract callers
// should range over for routing logic.
func (p *ServiceProvider) Keys() []string {
	if p == nil {
		return nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	keys := make([]string, 0, len(p.services))
	for k := range p.services {
		keys = append(keys, k)
	}
	return keys
}

// ServiceOf is the typed lookup helper. Returns the value registered at
// key cast to T, plus an ok flag. The flag is false when the key is
// missing OR the registered value is not assignable to T — callers can
// distinguish the two by checking [ServiceProvider.Has] separately when
// it matters.
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
