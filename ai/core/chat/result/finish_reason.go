package result

// FinishReason is a custom string type that represents the reason why a chat interaction
// or process has finished. It provides a set of predefined constants to categorize
// the different reasons for completion.
type FinishReason string

func (r FinishReason) String() string {
	return string(r)
}

// Constants:
//
// Stop          FinishReason = "stop"
//   - Indicates that the chat interaction was stopped intentionally, possibly by the user or system.
//
// Length        FinishReason = "length"
//   - Indicates that the chat interaction finished due to reaching a predefined length limit,
//     such as a maximum number of tokens or characters.
//
// ToolCalls     FinishReason = "tool_calls"
//   - Indicates that the chat interaction finished due to the invocation of external tools or APIs,
//     which may have provided the necessary response or action.
//
// ContentFilter FinishReason = "content_filter"
//   - Indicates that the chat interaction was terminated due to content filtering,
//     possibly because the content violated certain guidelines or policies.
//
// Other         FinishReason = "other"
//   - Represents any other reason for the chat interaction's completion that does not fit into the predefined categories.
//
// Null          FinishReason = "null"
//   - Represents a null or undefined finish reason, possibly used as a default or placeholder value.
const (
	Stop          FinishReason = "stop"
	Length        FinishReason = "length"
	ToolCalls     FinishReason = "tool_calls"
	ContentFilter FinishReason = "content_filter"
	Other         FinishReason = "other"
	Null          FinishReason = "null"
)

func (r FinishReason) IsStop() bool {
	return r == Stop
}

func (r FinishReason) IsLength() bool {
	return r == Length
}

func (r FinishReason) IsToolCalls() bool {
	return r == ToolCalls
}

func (r FinishReason) IsContentFilter() bool {
	return r == ContentFilter
}

func (r FinishReason) IsOther() bool {
	return r == Other
}

func (r FinishReason) IsNull() bool {
	return r == Null
}
