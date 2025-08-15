package tool

import "github.com/Tangerg/lynx/ai/model/chat"

// Options extends chat.Options with tool-specific configuration for LLM function calling.
// Provides a unified interface for managing both standard chat parameters and tool settings.
//
// Key capabilities:
// - Configure available tools for function calling
// - Set execution parameters for tool invocation
// - Support both internal (auto-executed) and external (client-delegated) tools
// - Maintain compatibility with standard chat options
type Options interface {
	chat.Options // Standard chat configuration (model, temperature, etc.)

	// Tools returns the list of available tools for LLM function calling.
	// The returned slice should be treated as read-only.
	Tools() []Tool

	// SetTools replaces all available tools for LLM function calling.
	// Accepts both internal and external tools.
	SetTools(tools []Tool)

	// AddTools appends additional tools to the existing tool list.
	// Accepts both internal and external tools.
	AddTools(tools []Tool)

	// ToolParams returns additional parameters passed to tools during execution.
	// These provide contextual information beyond function arguments.
	//
	// Common parameter examples:
	// - API endpoints and base URLs
	// - Timeout values and retry settings
	// - Authentication tokens
	// - Environment configuration (dev/prod)
	// - User context (user ID, session data)
	//
	// The returned map should be treated as read-only.
	ToolParams() map[string]any

	// SetToolParams replaces all tool execution parameters.
	// Parameters are passed to the tool execution Context.
	SetToolParams(params map[string]any)

	// AddToolParams adds parameters to existing tool parameters.
	// If a key already exists, it will be overwritten.
	AddToolParams(params map[string]any)
}
