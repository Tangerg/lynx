package anthropic

import (
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"

	corechat "github.com/Tangerg/lynx/core/chat"
)

// RequestDialect owns provider-specific request semantics layered on top of
// Anthropic's standard Messages wire shape. Source is read-only;
// implementations may mutate only target and must be safe for concurrent use
// when their Chat is shared.
type RequestDialect interface {
	PrepareRequest(source *corechat.Request, target *anthropicsdk.MessageNewParams) error
}

// ResponseDialect owns provider-specific response semantics for complete
// messages and streaming events. Source is read-only; implementations may
// mutate only target and must be safe for concurrent use when their Chat is
// shared.
type ResponseDialect interface {
	FinalizeMessage(source *anthropicsdk.Message, target *corechat.Response) error
	FinalizeEvent(source anthropicsdk.MessageStreamEventUnion, target *corechat.Response) error
}

// Dialect groups the independently typed request and response protocol facets
// selected by an Anthropic-compatible provider adapter.
type Dialect struct {
	Request  RequestDialect
	Response ResponseDialect
}
