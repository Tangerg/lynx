package openai

import (
	"fmt"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	corechat "github.com/Tangerg/lynx/core/chat"
)

func applyChatOutputFormat(format *corechat.OutputFormat, target *openaisdk.ChatCompletionNewParams, dialect Dialect) error {
	if format == nil {
		return nil
	}
	if dialect.NativeOutputFormat != nil && !dialect.NativeOutputFormat(format.Type) {
		instruction, err := format.FallbackInstruction()
		if err != nil {
			return fmt.Errorf("openai: output format fallback: %w", err)
		}
		if instruction != "" {
			target.Messages = append([]openaisdk.ChatCompletionMessageParamUnion{openaisdk.SystemMessage(instruction)}, target.Messages...)
		}
		return nil
	}

	responseFormat, err := mapChatOutputFormat(format)
	if err != nil {
		return err
	}
	target.ResponseFormat = responseFormat
	return nil
}

func mapChatOutputFormat(format *corechat.OutputFormat) (openaisdk.ChatCompletionNewParamsResponseFormatUnion, error) {
	switch format.Type {
	case corechat.OutputFormatText:
		return openaisdk.ChatCompletionNewParamsResponseFormatUnion{
			OfText: &shared.ResponseFormatTextParam{},
		}, nil
	case corechat.OutputFormatJSON:
		return openaisdk.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}, nil
	case corechat.OutputFormatJSONSchema:
		schema, err := format.SchemaAs[map[string]any]()
		if err != nil {
			return openaisdk.ChatCompletionNewParamsResponseFormatUnion{}, fmt.Errorf("openai: output schema: %w", err)
		}
		definition := shared.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   format.Name,
			Schema: schema,
			Strict: openaisdk.Bool(true),
		}
		if format.Description != "" {
			definition.Description = openaisdk.String(format.Description)
		}
		return openaisdk.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{JSONSchema: definition},
		}, nil
	default:
		return openaisdk.ChatCompletionNewParamsResponseFormatUnion{}, fmt.Errorf("openai: unsupported output format %q", format.Type)
	}
}
