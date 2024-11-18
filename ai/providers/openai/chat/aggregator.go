package chat

import (
	xslices "github.com/Tangerg/lynx/pkg/slices"
	"github.com/sashabaranov/go-openai"
)

type streamAggregator struct {
	chunks []*openai.ChatCompletionStreamResponse
}

func newAggregator() *streamAggregator {
	return &streamAggregator{
		chunks: make([]*openai.ChatCompletionStreamResponse, 0),
	}
}

func (a *streamAggregator) addChunk(streamChunk *openai.ChatCompletionStreamResponse) {
	a.chunks = append(a.chunks, streamChunk)
}

func (a *streamAggregator) aggregate() *openai.ChatCompletionResponse {
	if len(a.chunks) == 0 {
		return &openai.ChatCompletionResponse{}
	}
	rv := &openai.ChatCompletionResponse{
		ID:                a.chunks[0].ID,
		Object:            a.chunks[0].Object,
		Created:           a.chunks[0].Created,
		Model:             a.chunks[0].Model,
		SystemFingerprint: a.chunks[0].SystemFingerprint,
		Choices:           a.aggregateChoices(),
	}

	if a.chunks[len(a.chunks)-1].Usage != nil {
		rv.Usage = *a.chunks[len(a.chunks)-1].Usage
	}

	return rv
}

func (a *streamAggregator) aggregateChoices() []openai.ChatCompletionChoice {
	rv := make([]openai.ChatCompletionChoice, 0)
	for _, chunk := range a.chunks {
		for _, choice := range chunk.Choices {
			rv = xslices.ExpandToFit(rv, choice.Index)
			rvChoice := rv[choice.Index]
			rvChoice.Index = choice.Index
			rvChoice.FinishReason = choice.FinishReason
			if choice.Delta.Role != "" {
				rvChoice.Message.Role = choice.Delta.Role
			}
			rvChoice.Message.Content += choice.Delta.Content

			for j := range choice.Delta.ToolCalls {
				deltaTool := &choice.Delta.ToolCalls[j]

				rvChoice.Message.ToolCalls = xslices.ExpandToFit(rvChoice.Message.ToolCalls, *deltaTool.Index)
				tool := &rvChoice.Message.ToolCalls[*deltaTool.Index]

				if deltaTool.ID != "" {
					tool.ID = deltaTool.ID
				}
				if deltaTool.Type != "" {
					tool.Type = deltaTool.Type
				}
				tool.Function.Name += deltaTool.Function.Name
				tool.Function.Arguments += deltaTool.Function.Arguments
			}
		}
	}
	return rv
}
