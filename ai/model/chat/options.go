package chat

import (
	"github.com/Tangerg/lynx/ai/model"
)

// Options defines the configuration parameters for AI LLM chat models.
// These parameters control the behavior and output characteristics of large language models.
// All parameters are optional and use pointers to distinguish between zero values and unset values.
type Options interface {
	model.Options

	// FrequencyPenalty reduces repetition in LLM output by penalizing frequently used tokens.
	// Range: typically -2.0 to 2.0, where positive values decrease repetition.
	FrequencyPenalty() *float64

	// MaxTokens limits the maximum number of tokens the LLM can generate in response.
	// This controls the length and computational cost of the generated text.
	MaxTokens() *int64

	// PresencePenalty encourages the LLM to introduce new topics and concepts.
	// Range: typically -2.0 to 2.0, where positive values promote topic diversity.
	PresencePenalty() *float64

	// Stop defines text sequences that will halt LLM generation when encountered.
	// Useful for controlling output format and preventing unwanted continuation.
	Stop() []string

	// Temperature controls the randomness of LLM token selection.
	// Range: typically 0.0 to 2.0, where 0 is deterministic and higher values increase creativity.
	Temperature() *float64

	// TopK limits the LLM to consider only the K most probable next tokens.
	// Lower values make output more focused, higher values allow more diversity.
	TopK() *int64

	// TopP implements nucleus sampling for LLM token selection.
	// Range: 0.0 to 1.0, considers tokens with cumulative probability up to P.
	TopP() *float64

	// Clone creates a deep copy of these LLM configuration options.
	Clone() Options
}

// ToolOptions extends Options with tool-specific configuration for LLM function calling.
// Provides a unified interface for managing both standard chat parameters and tool settings.
//
// Key capabilities:
// - Configure available tools for function calling
// - Set execution parameters for tool invocation
// - Support both internal (auto-executed) and external (client-delegated) tools
// - Maintain compatibility with standard chat options
type ToolOptions interface {
	Options // Standard chat configuration (model, temperature, etc.)

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
