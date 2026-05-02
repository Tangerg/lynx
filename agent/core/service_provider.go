package core

import "context"

// ChatClient is the agent's interface to an LLM. We intentionally keep the
// surface minimal — actions usually want to call a chat client they already
// own, not learn the agent framework's chat surface. Providers ARE expected
// to provide concrete clients; we wrap them via this duck-typed interface so
// the agent module stays free of a hard dependency on lynx/core.
type ChatClient interface {
	// Generate is the synchronous "give me a single response" entry point. The
	// implementation is responsible for tool dispatching, structured output
	// parsing, etc. — the agent layer never inspects the result beyond using
	// it as a string or as an opaque payload.
	Generate(ctx context.Context, prompt string) (string, error)
}

// ChatClientFunc lifts a plain function into the ChatClient interface — so
// trivial cases (mocks, local stubs) don't need to define a type.
type ChatClientFunc func(ctx context.Context, prompt string) (string, error)

func (f ChatClientFunc) Generate(ctx context.Context, prompt string) (string, error) {
	return f(ctx, prompt)
}

// RAGClient is the optional retrieval-augmented-generation surface. Mirrors
// ChatClient's stance: opaque enough to pass through, typed enough to use.
type RAGClient interface {
	Retrieve(ctx context.Context, query string) ([]RetrievedDoc, error)
}

// RetrievedDoc is the framework-internal "document with metadata" carrier.
// Concrete RAG providers fill in whatever metadata they support.
type RetrievedDoc struct {
	ID       string
	Text     string
	Score    float64
	Metadata map[string]any
}

// VectorStore is the embed-and-search surface. Like the others, intentionally
// minimal — actions that need richer queries ought to import the concrete
// store directly instead of going through this façade.
type VectorStore interface {
	Search(ctx context.Context, query string, k int) ([]RetrievedDoc, error)
}

// ServiceProvider is the bag of optional integrations the platform makes
// available to actions. Any field can be nil — actions check before using.
type ServiceProvider struct {
	Chat        ChatClient
	RAG         RAGClient
	VectorStore VectorStore
	Tools       ToolGroupResolver
}

// NewServiceProvider returns an empty ServiceProvider; callers populate
// fields as needed. Provided for completeness — the zero value works too.
func NewServiceProvider() *ServiceProvider { return &ServiceProvider{} }
