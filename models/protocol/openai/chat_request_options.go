package openai

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	openaisdk "github.com/openai/openai-go/v3"

	corechat "github.com/Tangerg/scope/core/chat"
)

func (c *Chat) applyRequestExtension(req *corechat.Request, params *openaisdk.ChatCompletionNewParams) error {
	if c.dialect.DisableRawRequestExtension {
		return nil
	}

	extensionKey := protocolRequestExtensionKey(c.dialect.Provider)
	fields, err := decodeRequestFields(req.Options.Extensions, extensionKey,
		"model", "messages", "tools", "frequency_penalty", "max_tokens",
		"max_completion_tokens", "presence_penalty", "reasoning_effort", "response_format", "stop", "temperature", "top_p",
	)
	if err != nil {
		return err
	}
	if _, exists := fields["n"]; exists {
		return fmt.Errorf("openai: extension %q field %q is unsupported; Core Chat produces one output", extensionKey, "n")
	}
	params.SetExtraFields(fields)
	return nil
}

func (c *Chat) applyOptions(options corechat.Options, params *openaisdk.ChatCompletionNewParams) error {
	if options.Model == "" {
		return errors.New("openai: model is required in defaults or request options")
	}
	if options.TopK != nil {
		return errors.New("openai: options.top_k is not supported by Chat Completions")
	}
	params.Model = openaisdk.ChatModel(options.Model)
	if options.FrequencyPenalty != nil {
		params.FrequencyPenalty = openaisdk.Float(*options.FrequencyPenalty)
	}
	if err := c.applyTokenLimit(options.MaxTokens, params); err != nil {
		return err
	}
	if options.PresencePenalty != nil {
		params.PresencePenalty = openaisdk.Float(*options.PresencePenalty)
	}
	reasoningEffort, err := mapReasoningEffort(options.ReasoningEffort)
	if err != nil {
		return err
	}
	params.ReasoningEffort = reasoningEffort
	if len(options.Stop) > 0 {
		params.Stop.OfStringArray = slices.Clone(options.Stop)
	}
	if options.Temperature != nil {
		params.Temperature = openaisdk.Float(*options.Temperature)
	}
	if options.TopP != nil {
		params.TopP = openaisdk.Float(*options.TopP)
	}
	return nil
}

func (c *Chat) applyTokenLimit(limit *int64, params *openaisdk.ChatCompletionNewParams) error {
	if limit == nil {
		return nil
	}
	switch c.dialect.TokenLimitField {
	case TokenLimitMaxTokens:
		params.MaxTokens = openaisdk.Int(*limit)
	case TokenLimitMaxCompletionTokens:
		params.MaxCompletionTokens = openaisdk.Int(*limit)
	default:
		return errors.New("openai: invalid max token field configuration")
	}
	return nil
}

func (c *Chat) prepareRequest(req *corechat.Request, stream bool, params *openaisdk.ChatCompletionNewParams) error {
	if c.dialect.request != nil {
		if err := c.dialect.request.PrepareRequest(req, params); err != nil {
			return fmt.Errorf("openai: request dialect: %w", err)
		}
	}
	if c.dialect.PrepareRequest == nil {
		return nil
	}

	compatible := &CompatibleRequest{
		model:       string(params.Model),
		stream:      stream,
		extraFields: maps.Clone(params.ExtraFields()),
	}
	if params.Temperature.Valid() {
		compatible.temperature = &params.Temperature.Value
	}
	if err := c.dialect.PrepareRequest(req, compatible); err != nil {
		return fmt.Errorf("openai: compatible request dialect: %w", err)
	}
	params.SetExtraFields(compatible.extraFields)
	return nil
}
