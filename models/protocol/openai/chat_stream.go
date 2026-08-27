package openai

import (
	"fmt"

	openaisdk "github.com/openai/openai-go/v3"

	corechat "github.com/Tangerg/lynx/core/chat"
)

type openAIStreamTool struct {
	id               string
	name             string
	pendingArguments string
}

type openAIStreamState struct {
	tools       map[int64]openAIStreamTool
	dialect     responseDialect
	chunkKey    string
	refusalKey  string
	refusalPart string
}

func newOpenAIStreamState(dialect Dialect) *openAIStreamState {
	return &openAIStreamState{
		tools:       make(map[int64]openAIStreamTool),
		dialect:     dialect.response,
		chunkKey:    protocolStreamChunkExtensionKey(dialect.Provider),
		refusalKey:  protocolRefusalDeltaExtensionKey(dialect.Provider),
		refusalPart: protocolRefusalExtensionKey(dialect.Provider),
	}
}

func (o *openAIStreamState) mapChunk(chunk openaisdk.ChatCompletionChunk) (*corechat.Response, error) {
	mapped := &corechat.Response{
		Metadata: &corechat.ResponseMetadata{
			ID:    chunk.ID,
			Model: chunk.Model,
			Usage: mapUsage(chunk.Usage),
		},
	}
	if len(chunk.Choices) > 1 {
		return nil, fmt.Errorf("openai: stream chunk has %d choices; Core supports one output", len(chunk.Choices))
	}
	if len(chunk.Choices) == 1 {
		output, include, err := o.mapChunkOutput(chunk.Choices[0])
		if err != nil {
			return nil, fmt.Errorf("openai: stream output: %w", err)
		}
		if include {
			mapped.Output = output
		}
	}
	if err := mapped.Metadata.Set(o.chunkKey, exactProviderResponse(chunk.RawJSON(), chunk)); err != nil {
		return nil, err
	}
	if err := mapped.Validate(); err != nil {
		return nil, fmt.Errorf("openai: mapped stream response: %w", err)
	}
	return mapped, nil
}

func (o *openAIStreamState) mapChunkOutput(choice openaisdk.ChatCompletionChunkChoice) (*corechat.Output, bool, error) {
	if choice.Index != 0 {
		return nil, false, fmt.Errorf("choice index is %d, want 0", choice.Index)
	}
	mapped := &corechat.Output{FinishReason: normalizeFinishReason(choice.FinishReason)}
	parts := make([]corechat.Part, 0, 2+len(choice.Delta.ToolCalls))
	if choice.Delta.Content != "" {
		parts = append(parts, corechat.NewTextPart(choice.Delta.Content))
	}
	for i := range choice.Delta.ToolCalls {
		call, include, err := o.mapChunkTool(choice.Delta.ToolCalls[i])
		if err != nil {
			return nil, false, fmt.Errorf("tool_calls[%d]: %w", i, err)
		}
		if include {
			parts = append(parts, corechat.NewToolCallPart(call))
		}
	}
	message := &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
	if o.dialect != nil {
		if err := o.dialect.FinalizeDelta(choice.Delta, message); err != nil {
			return nil, false, fmt.Errorf("response dialect: %w", err)
		}
	}
	if len(message.Parts) > 0 {
		if choice.Delta.Refusal != "" {
			if err := message.Metadata.Set(o.refusalPart, choice.Delta.Refusal); err != nil {
				return nil, false, err
			}
		}
		mapped.Message = message
	} else if choice.Delta.Refusal != "" {
		mapped.Metadata = &corechat.OutputMetadata{}
		if err := mapped.Metadata.Set(o.refusalKey, choice.Delta.Refusal); err != nil {
			return nil, false, err
		}
	}

	include := mapped.Message != nil || mapped.FinishReason != "" || mapped.Metadata != nil
	return mapped, include, nil
}

func (o *openAIStreamState) mapChunkTool(delta openaisdk.ChatCompletionChunkChoiceDeltaToolCall) (corechat.ToolCall, bool, error) {
	if delta.Type != "" && delta.Type != "function" {
		return corechat.ToolCall{}, false, fmt.Errorf("unsupported type %q", delta.Type)
	}
	state := o.tools[delta.Index]
	if delta.ID != "" {
		state.id = delta.ID
	}
	if delta.Function.Name != "" {
		state.name = delta.Function.Name
	}
	state.pendingArguments += delta.Function.Arguments
	o.tools[delta.Index] = state
	if state.id == "" || state.name == "" {
		return corechat.ToolCall{}, false, nil
	}
	arguments := state.pendingArguments
	state.pendingArguments = ""
	o.tools[delta.Index] = state
	return corechat.ToolCall{
		ID:        state.id,
		Name:      state.name,
		Arguments: arguments,
	}, true, nil
}
