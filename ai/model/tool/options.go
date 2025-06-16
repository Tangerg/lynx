package tool

import "github.com/Tangerg/lynx/ai/model/chat"

// Options extends chat.Options with tool-specific configuration for LLM interactions
// that support function calling. It provides a unified interface for managing both
// standard chat parameters and tool-related settings in a single configuration object.
//
// This interface enables LLM clients to:
// - Configure available tools for function calling
// - Set execution parameters passed to tools during invocation
// - Maintain compatibility with standard chat options
// - Support both internal tools (executed immediately) and external tools (client-delegated)
//
// Implementations should ensure thread-safety if used across multiple goroutines.
type Options interface {
	chat.Options // Inherits standard chat configuration (model, temperature, etc.)

	// Tools returns the list of tools available for LLM function calling.
	// The returned slice should be treated as read-only to prevent unintended modifications.
	// Tools can be either internal (CallableTool) or external (require client execution).
	Tools() []Tool

	// SetTools configures the available tools for LLM function calling.
	// The provided tools will be serialized and sent to the LLM as function definitions.
	// Accepts both internal tools (executed automatically) and external tools (delegated to client).
	//
	// Parameters:
	//   - tools: Slice of Tool instances to make available for function calling.
	SetTools(tools []Tool)

	// Params returns additional parameters that will be passed to tools during execution.
	// These parameters provide contextual information or configuration that tools may need
	// beyond their specific function arguments.
	//
	// Examples of tool execution parameters:
	// - API endpoints or base URLs for external services
	// - Timeout values for tool operations
	// - Authentication tokens or credentials
	// - Environment-specific configuration (dev/prod settings)
	// - User context information (user ID, session data)
	//
	// The returned map should be treated as read-only.
	Params() map[string]any

	// SetParams configures additional parameters that will be available to tools during execution.
	// These parameters are passed to the tool execution context and can be accessed by tools
	// to customize their behavior or access external resources.
	//
	// Parameters:
	//   - params: Map of parameter names to values that tools can access during execution.
	//            Parameter names and types depend on the specific tools being used.
	SetParams(params map[string]any)
}
