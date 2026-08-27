package mistral

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	corechat "github.com/Tangerg/lynx/core/chat"
)

type chatStreamTool struct {
	id               string
	name             string
	pendingArguments string
}

type chatStreamState struct {
	tools map[int]chatStreamTool
}

func newChatStreamState() *chatStreamState {
	return &chatStreamState{tools: make(map[int]chatStreamTool)}
}

func (c *chatStreamState) mapChunk(chunk chatCompletionChunk) (*corechat.Response, error) {
	response := &corechat.Response{
		Metadata: &corechat.ResponseMetadata{
			ID: chunk.ID, Model: chunk.Model, Usage: mapMistralUsage(chunk.Usage),
		},
	}
	if err := response.Metadata.Set(streamChunkExtensionKey, chunk); err != nil {
		return nil, err
	}
	if len(chunk.Choices) > 1 {
		return nil, fmt.Errorf("mistral: stream chunk has %d choices; Core supports one output", len(chunk.Choices))
	}
	if len(chunk.Choices) == 1 {
		wireChoice := chunk.Choices[0]
		if wireChoice.Index != 0 {
			return nil, fmt.Errorf("mistral: stream choice index is %d, want 0", wireChoice.Index)
		}
		parts, err := mapMistralContent(wireChoice.Delta.Content)
		if err != nil {
			return nil, fmt.Errorf("mistral: stream output content: %w", err)
		}
		toolParts, err := c.mapToolDeltas(wireChoice.Delta.ToolCalls)
		if err != nil {
			return nil, fmt.Errorf("mistral: stream output tool calls: %w", err)
		}
		parts = append(parts, toolParts...)
		response.Output = &corechat.Output{
			FinishReason: normalizeMistralFinishReason(wireChoice.FinishReason),
		}
		if len(parts) > 0 {
			response.Output.Message = &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
		}
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("mistral: mapped stream chunk: %w", err)
	}
	return response, nil
}

func (c *chatStreamState) mapToolDeltas(calls []chatToolCall) ([]corechat.Part, error) {
	parts := make([]corechat.Part, 0, len(calls))
	for position := range calls {
		call := calls[position]
		index := call.Index
		tool := c.tools[index]
		if call.ID != "" {
			tool.id = call.ID
		}
		if call.Function.Name != "" {
			tool.name = call.Function.Name
		}
		arguments, err := mistralToolArguments(call.Function.Arguments)
		if err != nil {
			return nil, fmt.Errorf("tool call %d arguments: %w", index, err)
		}
		tool.pendingArguments += arguments
		c.tools[index] = tool
		if tool.id == "" || tool.name == "" {
			continue
		}
		deltaArguments := tool.pendingArguments
		tool.pendingArguments = ""
		c.tools[index] = tool
		parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{
			ID: tool.id, Name: tool.name, Arguments: deltaArguments,
		}))
	}
	return parts, nil
}

func scanMistralSSE(reader io.Reader, yield func([]byte) bool) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var data []byte
	flush := func() bool {
		if len(data) == 0 {
			return true
		}
		payload := bytes.TrimSpace(data)
		data = data[:0]
		if bytes.Equal(payload, []byte("[DONE]")) {
			return false
		}
		return yield(payload)
	}
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			if !flush() {
				return nil
			}
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, bytes.TrimSpace(line[len("data:"):])...)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mistral: read chat stream: %w", err)
	}
	flush()
	return nil
}
