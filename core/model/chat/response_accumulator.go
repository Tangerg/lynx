package chat

import (
	"maps"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// ResponseAccumulator accumulates streaming chat response chunks into a complete response.
// It progressively merges multiple response fragments received during streaming,
// combining text content, tool calls, metadata, and other response components.
type ResponseAccumulator struct {
	Response
}

// NewResponseAccumulator creates a new response accumulator instance.
// The accumulator starts with an empty response that will be progressively filled.
func NewResponseAccumulator() *ResponseAccumulator {
	return &ResponseAccumulator{}
}

// AddChunk adds a new response chunk to the accumulator.
// This is the main entry point for accumulating streaming responses.
// It merges both the results and metadata from the incoming chunk.
func (r *ResponseAccumulator) AddChunk(resp *Response) {
	r.accumulateResults(resp.Results)
	r.accumulateMetadata(resp.Metadata)
}

// accumulateMetadata merges metadata from a response chunk into the accumulated response.
// Strategy:
//   - Scalar fields (ID, Model, Usage, etc.) are overwritten with the latest values
//   - Extra map is merged, with newer values overwriting existing ones
func (r *ResponseAccumulator) accumulateMetadata(other *ResponseMetadata) {
	if other == nil {
		return
	}
	if r.Metadata == nil {
		r.Metadata = &ResponseMetadata{}
	}
	// Overwrite with latest values
	r.Metadata.ID = other.ID
	r.Metadata.Model = other.Model
	r.Metadata.Usage = other.Usage
	r.Metadata.RateLimit = other.RateLimit
	r.Metadata.Created = other.Created

	// Merge extra metadata map
	r.Metadata.ensureExtra()
	maps.Copy(r.Metadata.Extra, other.Extra)
}

// accumulateResults merges an array of results from a response chunk.
// It ensures the results slice has sufficient capacity and accumulates
// each result at its corresponding index position.
func (r *ResponseAccumulator) accumulateResults(others []*Result) {
	if len(others) == 0 {
		return
	}
	// Ensure results slice has enough capacity for all incoming results
	r.Results = pkgSlices.EnsureIndex(r.Results, len(others)-1)
	for index, other := range others {
		r.accumulateResult(index, other)
	}
}

// accumulateResult merges a single result into the accumulator at the specified index.
// It handles three components:
//   - AssistantMessage: accumulated via string concatenation for streaming text
//   - ResultMetadata: merged with latest values
//   - ToolMessage: accumulated or overwritten depending on the component
func (r *ResponseAccumulator) accumulateResult(index int, other *Result) {
	result := r.Results[index]
	if result == nil {
		result = &Result{}
		r.Results[index] = result
	}
	result.AssistantMessage = r.accumulateAssistantMessage(result.AssistantMessage, other.AssistantMessage)
	result.Metadata = r.accumulateResultMetadata(result.Metadata, other.Metadata)
	result.ToolMessage = r.accumulateToolMessage(result.ToolMessage, other.ToolMessage)
}

// accumulateAssistantMessage merges assistant message content from streaming chunks.
// Accumulation strategy:
//   - Text: concatenated (e.g., "Hello" + " world" = "Hello world")
//   - ToolCalls: each component (ID, Name, Arguments) is concatenated
//     This supports streaming tool calls where JSON is sent in fragments
//   - Metadata: merged with newer values overwriting existing ones
func (r *ResponseAccumulator) accumulateAssistantMessage(msg, other *AssistantMessage) *AssistantMessage {
	if other == nil {
		return msg
	}
	if msg == nil {
		msg = &AssistantMessage{}
	}

	// Concatenate text content for streaming generation
	msg.Text += other.Text

	// Accumulate tool calls by concatenating their components
	if len(other.ToolCalls) > 0 {
		msg.ToolCalls = pkgSlices.EnsureIndex(msg.ToolCalls, len(other.ToolCalls)-1)
		for index, toolCall := range other.ToolCalls {
			tc := msg.ToolCalls[index]
			if tc == nil {
				tc = &ToolCall{}
				msg.ToolCalls[index] = tc
			}
			// Concatenate tool call components (supports chunked transmission)
			tc.ID += toolCall.ID
			tc.Name += toolCall.Name
			tc.Arguments += toolCall.Arguments
		}
	}

	maps.Copy(msg.Meta(), other.Metadata)

	return msg
}

// accumulateToolMessage merges tool execution results into the accumulated message.
// Strategy: ToolReturns are overwritten rather than accumulated because they represent
// complete, atomic results from tool execution rather than streaming fragments.
func (r *ResponseAccumulator) accumulateToolMessage(msg, other *ToolMessage) *ToolMessage {
	if other == nil {
		return msg
	}
	if msg == nil {
		msg = &ToolMessage{}
	}

	// Ensure tool returns slice has sufficient capacity
	if len(other.ToolReturns) > 0 {
		msg.ToolReturns = pkgSlices.EnsureIndex(msg.ToolReturns, len(other.ToolReturns)-1)
		for index, toolReturn := range other.ToolReturns {
			tr := msg.ToolReturns[index]
			if tr == nil {
				tr = &ToolReturn{}
				msg.ToolReturns[index] = tr
			}
			// Direct overwrite: tool returns are complete one-time generations
			// rather than streaming fragments, so no accumulation is needed
			tr.ID = toolReturn.ID
			tr.Name = toolReturn.Name
			tr.Result = toolReturn.Result
		}

	}

	maps.Copy(msg.Meta(), other.Metadata)

	return msg
}

// accumulateResultMetadata merges result-level metadata from a response chunk.
// Strategy:
//   - FinishReason: overwritten (typically only present in the final chunk)
//   - Extra: merged with newer values overwriting existing ones
func (r *ResponseAccumulator) accumulateResultMetadata(meta, other *ResultMetadata) *ResultMetadata {
	if other == nil {
		return meta
	}

	if meta == nil {
		meta = &ResultMetadata{}
	}
	// Overwrite finish reason (usually only set in the last chunk)
	meta.FinishReason = other.FinishReason

	// Merge extra metadata
	meta.ensureExtra()
	maps.Copy(meta.Extra, other.Extra)

	return meta
}
