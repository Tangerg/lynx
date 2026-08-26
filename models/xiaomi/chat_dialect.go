package xiaomi

import (
	"errors"
	"fmt"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/protocol/openai"
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

func (c ChatRequestOptions) Validate() error {
	switch c.Thinking {
	case "", ThinkingEnabled, ThinkingDisabled:
		return nil
	default:
		return fmt.Errorf("thinking must be %q or %q", ThinkingEnabled, ThinkingDisabled)
	}
}

func prepareOpenAIRequest(source *corechat.Request, target *openai.CompatibleRequest) error {
	if temperature, ok := target.Temperature(); ok && temperature > 1.5 {
		return errors.New("xiaomi: temperature must be between 0 and 1.5")
	}
	options, found, err := source.Options.Extensions.Decode[ChatRequestOptions](RequestExtensionKey)
	if err != nil {
		return fmt.Errorf("xiaomi: extension %q: %w", RequestExtensionKey, err)
	}
	if !found {
		return nil
	}
	if err := options.Validate(); err != nil {
		return fmt.Errorf("xiaomi: extension %q: %w", RequestExtensionKey, err)
	}
	if options.Thinking == "" {
		return nil
	}
	return target.SetExtraField("thinking", map[string]any{"type": options.Thinking})
}
