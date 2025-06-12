package result

// FinishReason represents the reason why a chat completion has finished.
// It uses a custom string type to provide type safety and predefined constants
// for different completion scenarios.
type FinishReason string

func (r FinishReason) String() string {
	return string(r)
}

// Predefined finish reasons for chat completions
const (
	// Stop indicates the model completed the response naturally or was explicitly stopped
	Stop FinishReason = "stop"

	// Length indicates the response was truncated due to maximum token/length limits
	Length FinishReason = "length"

	// ToolCalls indicates the model finished to execute function/tool calls
	ToolCalls FinishReason = "tool_calls"

	// ContentFilter indicates the response was blocked by content safety filters
	ContentFilter FinishReason = "content_filter"

	// ReturnDirect indicates the tool execution results were returned directly
	// without AI processing, typically when all executed tools have ReturnDirect=true
	ReturnDirect FinishReason = "return_direct"

	// Other represents any completion reason not covered by other constants
	Other FinishReason = "other"

	// Null represents an undefined or unset finish reason
	Null FinishReason = "null"
)

// IsStop returns true if the finish reason is Stop
func (r FinishReason) IsStop() bool {
	return r == Stop
}

// IsLength returns true if the finish reason is Length
func (r FinishReason) IsLength() bool {
	return r == Length
}

// IsToolCalls returns true if the finish reason is ToolCalls
func (r FinishReason) IsToolCalls() bool {
	return r == ToolCalls
}

// IsContentFilter returns true if the finish reason is ContentFilter
func (r FinishReason) IsContentFilter() bool {
	return r == ContentFilter
}

// IsReturnDirect returns true if the finish reason is ReturnDirect
func (r FinishReason) IsReturnDirect() bool {
	return r == ReturnDirect
}

// IsOther returns true if the finish reason is Other
func (r FinishReason) IsOther() bool {
	return r == Other
}

// IsNull returns true if the finish reason is Null
func (r FinishReason) IsNull() bool {
	return r == Null
}
