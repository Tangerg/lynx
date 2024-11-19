package middleware

import (
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// Middleware represents a middleware function in a chat application, enabling the processing
// and manipulation of chat requests and responses within a middleware chain. It operates
// on a context that carries chat-specific options and metadata.
//
// Type Parameters:
//   - O: Represents the chat options, defined by request.ChatRequestOptions.
//   - M: Represents the metadata associated with the chat generation, defined by result.ChatResultMetadata.
//
// Parameters:
//   - ctx: A pointer to the Context[O, M] instance that carries the chat request, response,
//     and shared data. Middleware functions can use this context to modify or inspect
//     the request and response or to share data with other middleware components.
//
// Returns:
//   - error: An error indicating issues encountered during middleware execution. If the
//     middleware completes successfully, it should return nil.
//
// Usage:
// Middleware functions are chained together to process chat requests and responses sequentially.
// They are commonly used for tasks such as:
//   - Logging request and response details.
//   - Performing authentication and authorization checks.
//   - Transforming or validating requests before processing.
//   - Modifying responses before sending them to the client.
//
// Middleware in the chain can choose to:
//   - Pass control to the next middleware by calling `ctx.Next()`.
//   - Terminate the chain by returning an error.
//
// Example Middleware:
//
//	func LoggingMiddleware[O request.ChatRequestOptions, M result.ChatResultMetadata](ctx *Context[O, M]) error {
//	    log.Printf("Processing request: %v", ctx.Request)
//	    err := ctx.Next()
//	    if err != nil {
//	        log.Printf("Error encountered: %v", err)
//	        return err
//	    }
//	    log.Printf("Response generated: %v", ctx.Response)
//	    return nil
//	}
type Middleware[O request.ChatRequestOptions, M result.ChatResultMetadata] func(ctx *Context[O, M]) error
