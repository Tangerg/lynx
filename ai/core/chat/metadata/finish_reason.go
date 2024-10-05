package metadata

type FinishReason string

func (r FinishReason) String() string {
	return string(r)
}

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
