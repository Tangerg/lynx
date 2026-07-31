package xiaomi

import (
	"fmt"

	openaisdk "github.com/openai/openai-go/v3"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

const RequestExtensionKey = "xiaomi/request"

// ThinkingType controls MiMo deep thinking.
type ThinkingType string

const (
	ThinkingEnabled  ThinkingType = "enabled"
	ThinkingDisabled ThinkingType = "disabled"
)

// ChatRequestOptions contains MiMo Chat Completions fields without a
// provider-neutral Core equivalent.
type ChatRequestOptions struct {
	Thinking ThinkingType `json:"thinking,omitempty"`
}

func (options ChatRequestOptions) validate() error {
	switch options.Thinking {
	case "", ThinkingEnabled, ThinkingDisabled:
		return nil
	default:
		return fmt.Errorf("thinking must be %q or %q", ThinkingEnabled, ThinkingDisabled)
	}
}

type requestDialect struct {
	reasoning openAIRequestDialect
}

type openAIRequestDialect interface {
	PrepareRequest(source *corechat.Request, target *openaisdk.ChatCompletionNewParams) error
}

func (dialect requestDialect) PrepareRequest(source *corechat.Request, target *openaisdk.ChatCompletionNewParams) error {
	if dialect.reasoning != nil {
		if err := dialect.reasoning.PrepareRequest(source, target); err != nil {
			return err
		}
	}
	if target.Temperature.Valid() && target.Temperature.Value > 1.5 {
		return fmt.Errorf("xiaomi: temperature must be between 0 and 1.5")
	}
	options, found, err := metadata.Decode[ChatRequestOptions](source.Extensions, RequestExtensionKey)
	if err != nil {
		return fmt.Errorf("xiaomi: extension %q: %w", RequestExtensionKey, err)
	}
	if !found {
		return nil
	}
	if err := options.validate(); err != nil {
		return fmt.Errorf("xiaomi: extension %q: %w", RequestExtensionKey, err)
	}
	if options.Thinking == "" {
		return nil
	}
	extraFields := target.ExtraFields()
	merged := make(map[string]any, len(extraFields)+1)
	for key, value := range extraFields {
		merged[key] = value
	}
	merged["thinking"] = map[string]any{"type": options.Thinking}
	target.SetExtraFields(merged)
	return nil
}
