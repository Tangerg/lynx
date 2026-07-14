package google

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

const protocolRequestExtensionKey = "google/request"

func mapProtocolRequest(defaults corechat.Options, req *corechat.Request) (string, []*genai.Content, *genai.GenerateContentConfig, error) {
	config, err := decodeProtocolConfig(req)
	if err != nil {
		return "", nil, nil, err
	}
	options := mergeProtocolOptions(defaults, req.Options)
	if options.Model == "" {
		return "", nil, nil, errors.New("google: model is required in defaults or request options")
	}
	if options.MaxTokens != nil {
		if *options.MaxTokens > math.MaxInt32 {
			return "", nil, nil, errors.New("google: options.max_tokens exceeds int32")
		}
		config.MaxOutputTokens = int32(*options.MaxTokens)
	}
	if options.Temperature != nil {
		value := float32(*options.Temperature)
		config.Temperature = &value
	}
	if options.TopK != nil {
		value := float32(*options.TopK)
		config.TopK = &value
	}
	if options.TopP != nil {
		value := float32(*options.TopP)
		config.TopP = &value
	}
	if options.FrequencyPenalty != nil {
		value := float32(*options.FrequencyPenalty)
		config.FrequencyPenalty = &value
	}
	if options.PresencePenalty != nil {
		value := float32(*options.PresencePenalty)
		config.PresencePenalty = &value
	}
	if len(options.Stop) > 0 {
		config.StopSequences = slices.Clone(options.Stop)
	}

	system, contents, err := mapProtocolMessages(req.Messages)
	if err != nil {
		return "", nil, nil, err
	}
	if system != nil {
		if config.SystemInstruction == nil {
			config.SystemInstruction = system
		} else {
			config.SystemInstruction.Parts = append(config.SystemInstruction.Parts, system.Parts...)
		}
	}
	tools, err := mapProtocolTools(req.Tools)
	if err != nil {
		return "", nil, nil, err
	}
	config.Tools = append(config.Tools, tools...)
	return options.Model, contents, config, nil
}

func decodeProtocolConfig(req *corechat.Request) (*genai.GenerateContentConfig, error) {
	raw, found := req.Extensions[protocolRequestExtensionKey]
	if !found {
		return &genai.GenerateContentConfig{}, nil
	}
	var config genai.GenerateContentConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("google: extension %q: %w", protocolRequestExtensionKey, err)
	}
	var aliases struct {
		SafetySettings     []*genai.SafetySetting `json:"safety_settings"`
		ResponseModalities []string               `json:"response_modalities"`
	}
	if err := json.Unmarshal(raw, &aliases); err != nil {
		return nil, fmt.Errorf("google: extension %q aliases: %w", protocolRequestExtensionKey, err)
	}
	if len(config.SafetySettings) == 0 && len(aliases.SafetySettings) > 0 {
		config.SafetySettings = aliases.SafetySettings
	}
	if len(config.ResponseModalities) == 0 && len(aliases.ResponseModalities) > 0 {
		config.ResponseModalities = slices.Clone(aliases.ResponseModalities)
	}
	return &config, nil
}

func cloneProtocolOptions(options corechat.Options) corechat.Options {
	clone := options
	clone.Stop = slices.Clone(options.Stop)
	clone.FrequencyPenalty = cloneProtocolPointer(options.FrequencyPenalty)
	clone.MaxTokens = cloneProtocolPointer(options.MaxTokens)
	clone.PresencePenalty = cloneProtocolPointer(options.PresencePenalty)
	clone.Temperature = cloneProtocolPointer(options.Temperature)
	clone.TopK = cloneProtocolPointer(options.TopK)
	clone.TopP = cloneProtocolPointer(options.TopP)
	return clone
}

func cloneProtocolPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func mergeProtocolOptions(defaults, overrides corechat.Options) corechat.Options {
	merged := cloneProtocolOptions(defaults)
	if overrides.Model != "" {
		merged.Model = overrides.Model
	}
	if overrides.FrequencyPenalty != nil {
		merged.FrequencyPenalty = overrides.FrequencyPenalty
	}
	if overrides.MaxTokens != nil {
		merged.MaxTokens = overrides.MaxTokens
	}
	if overrides.PresencePenalty != nil {
		merged.PresencePenalty = overrides.PresencePenalty
	}
	if len(overrides.Stop) > 0 {
		merged.Stop = slices.Clone(overrides.Stop)
	}
	if overrides.Temperature != nil {
		merged.Temperature = overrides.Temperature
	}
	if overrides.TopK != nil {
		merged.TopK = overrides.TopK
	}
	if overrides.TopP != nil {
		merged.TopP = overrides.TopP
	}
	return merged
}

func mapProtocolMessages(messages []corechat.Message) (*genai.Content, []*genai.Content, error) {
	systemParts := make([]*genai.Part, 0)
	contents := make([]*genai.Content, 0, len(messages))
	for i := range messages {
		message := messages[i]
		switch message.Role {
		case corechat.RoleSystem:
			for j := range message.Parts {
				systemParts = append(systemParts, genai.NewPartFromText(message.Parts[j].Text))
			}
		case corechat.RoleUser:
			parts, err := mapProtocolUserParts(message.Parts)
			if err != nil {
				return nil, nil, fmt.Errorf("google: messages[%d]: %w", i, err)
			}
			contents = append(contents, genai.NewContentFromParts(parts, genai.RoleUser))
		case corechat.RoleAssistant:
			parts, err := mapProtocolAssistantParts(message.Parts)
			if err != nil {
				return nil, nil, fmt.Errorf("google: messages[%d]: %w", i, err)
			}
			contents = append(contents, genai.NewContentFromParts(parts, genai.RoleModel))
		case corechat.RoleTool:
			parts := make([]*genai.Part, 0, len(message.Parts))
			for j := range message.Parts {
				result := message.Parts[j].ToolResult
				parts = append(parts, &genai.Part{FunctionResponse: &genai.FunctionResponse{
					ID:       result.ID,
					Name:     result.Name,
					Response: protocolToolResult(result.Result, result.IsError),
				}})
			}
			contents = append(contents, genai.NewContentFromParts(parts, genai.RoleUser))
		default:
			return nil, nil, fmt.Errorf("google: messages[%d]: unsupported role %q", i, message.Role)
		}
	}
	var system *genai.Content
	if len(systemParts) > 0 {
		system = genai.NewContentFromParts(systemParts, "")
	}
	return system, contents, nil
}

func mapProtocolUserParts(parts []corechat.Part) ([]*genai.Part, error) {
	mapped := make([]*genai.Part, 0, len(parts))
	for i := range parts {
		switch parts[i].Kind {
		case corechat.PartText:
			mapped = append(mapped, genai.NewPartFromText(parts[i].Text))
		case corechat.PartMedia:
			part, err := mapProtocolMedia(parts[i].Media)
			if err != nil {
				return nil, fmt.Errorf("parts[%d]: %w", i, err)
			}
			mapped = append(mapped, part)
		default:
			return nil, fmt.Errorf("parts[%d]: unsupported user part %q", i, parts[i].Kind)
		}
	}
	return mapped, nil
}

func mapProtocolAssistantParts(parts []corechat.Part) ([]*genai.Part, error) {
	mapped := make([]*genai.Part, 0, len(parts))
	for i := range parts {
		part := parts[i]
		switch part.Kind {
		case corechat.PartText:
			mapped = append(mapped, genai.NewPartFromText(part.Text))
		case corechat.PartReasoning:
			mapped = append(mapped, &genai.Part{Text: part.Text, Thought: true, ThoughtSignature: slices.Clone(part.Signature)})
		case corechat.PartToolCall:
			var arguments map[string]any
			if part.ToolCall.Arguments != "" {
				if err := json.Unmarshal([]byte(part.ToolCall.Arguments), &arguments); err != nil {
					return nil, fmt.Errorf("parts[%d].tool_call.arguments: %w", i, err)
				}
			}
			mapped = append(mapped, &genai.Part{FunctionCall: &genai.FunctionCall{
				ID:   part.ToolCall.ID,
				Name: part.ToolCall.Name,
				Args: arguments,
			}})
		case corechat.PartMedia:
			mediaPart, err := mapProtocolMedia(part.Media)
			if err != nil {
				return nil, fmt.Errorf("parts[%d]: %w", i, err)
			}
			mapped = append(mapped, mediaPart)
		default:
			return nil, fmt.Errorf("parts[%d]: unsupported assistant part %q", i, part.Kind)
		}
	}
	return mapped, nil
}

func mapProtocolMedia(value *media.Media) (*genai.Part, error) {
	switch value.Source.Kind {
	case media.SourceBytes:
		data, err := value.Bytes()
		if err != nil {
			return nil, err
		}
		part := genai.NewPartFromBytes(data, value.MIME)
		part.InlineData.DisplayName = value.Name
		return part, nil
	case media.SourceURI:
		uri, err := value.URI()
		if err != nil {
			return nil, err
		}
		part := genai.NewPartFromURI(uri, value.MIME)
		part.FileData.DisplayName = value.Name
		return part, nil
	default:
		return nil, fmt.Errorf("media source %q is unsupported", value.Source.Kind)
	}
}

func protocolToolResult(result string, isError bool) map[string]any {
	var decoded any
	if result != "" && json.Unmarshal([]byte(result), &decoded) == nil {
		if !isError {
			if object, ok := decoded.(map[string]any); ok {
				return object
			}
			return map[string]any{"output": decoded}
		}
		return map[string]any{"error": decoded}
	}
	if isError {
		return map[string]any{"error": result}
	}
	return map[string]any{"output": result}
}

func mapProtocolTools(definitions []corechat.ToolDefinition) ([]*genai.Tool, error) {
	if len(definitions) == 0 {
		return nil, nil
	}
	declarations := make([]*genai.FunctionDeclaration, 0, len(definitions))
	for i := range definitions {
		var schema map[string]any
		if err := json.Unmarshal(definitions[i].InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("google: tools[%d].input_schema: %w", i, err)
		}
		declarations = append(declarations, &genai.FunctionDeclaration{
			Name:                 definitions[i].Name,
			Description:          definitions[i].Description,
			ParametersJsonSchema: schema,
		})
	}
	return []*genai.Tool{{FunctionDeclarations: declarations}}, nil
}
