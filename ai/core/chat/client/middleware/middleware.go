package middleware

import (
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// Middleware is a function type that represents a middleware component in a chat application.
// It takes a context of type *Context[O, M] as its parameter and returns an error.
//
// Type Parameters:
//   - O: Represents the chat options, which are defined by the prompt.ChatOptions type.
//   - M: Represents the metadata associated with the chat generation, defined by the metadata.ChatGenerationMetadata type.
//
// Parameters:
//   - ctx: A pointer to the Context[O, M] which holds the chat options and metadata. This context is used to pass
//     information and control flow between different middleware components.
//
// Returns:
//   - error: The function returns an error if the middleware encounters any issues during its execution.
//     If no error occurs, it should return nil.
//
// Usage:
// Middleware functions are used to process and manipulate chat requests and responses. They can be used for tasks
// such as logging, authentication, modifying request/response data, etc. Each middleware function can decide whether
// to pass control to the next middleware in the chain or terminate the chain by returning an error.
type Middleware[O request.ChatRequestOptions, M result.ChatResultMetadata] func(ctx *Context[O, M]) error
