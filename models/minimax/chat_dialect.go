package minimax

import (
	"errors"
	"fmt"

	openaisdk "github.com/openai/openai-go/v3"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

const (
	// RequestExtensionKey stores MiniMax-specific Chat Completions options in a
	// Core request.
	RequestExtensionKey = "minimax/request"
	reasoningSplitField = "reasoning_split"
)

// ThinkingType controls MiniMax-M3 thinking. M2.x models ignore disabled and
// always think according to the official protocol.
type ThinkingType string

const (
	ThinkingAdaptive ThinkingType = "adaptive"
	ThinkingDisabled ThinkingType = "disabled"
)

// ServiceTier controls MiniMax request admission priority.
type ServiceTier string

const (
	ServiceTierStandard ServiceTier = "standard"
	ServiceTierPriority ServiceTier = "priority"
)

// ChatRequestOptions contains MiniMax request fields without a provider-neutral
// Core equivalent.
type ChatRequestOptions struct {
	Thinking       ThinkingType `json:"thinking,omitempty"`
	ReasoningSplit *bool        `json:"reasoning_split,omitempty"`
	ServiceTier    ServiceTier  `json:"service_tier,omitempty"`
}

func (options ChatRequestOptions) validate() error {
	switch options.Thinking {
	case "", ThinkingAdaptive, ThinkingDisabled:
	default:
		return fmt.Errorf("thinking must be %q or %q", ThinkingAdaptive, ThinkingDisabled)
	}
	switch options.ServiceTier {
	case "", ServiceTierStandard, ServiceTierPriority:
	default:
		return fmt.Errorf("service_tier must be %q or %q", ServiceTierStandard, ServiceTierPriority)
	}
	return nil
}

// reasoningSplitDialect keeps MiniMax thinking separate from answer content.
// The official API otherwise embeds thinking in content inside <think> tags.
type reasoningSplitDialect struct{}

func (reasoningSplitDialect) PrepareRequest(source *corechat.Request, target *openaisdk.ChatCompletionNewParams) error {
	extension, found, err := metadata.Decode[ChatRequestOptions](source.Extensions, RequestExtensionKey)
	if err != nil {
		return fmt.Errorf("minimax: extension %q: %w", RequestExtensionKey, err)
	}
	if found {
		if err := extension.validate(); err != nil {
			return fmt.Errorf("minimax: extension %q: %w", RequestExtensionKey, err)
		}
	}
	reasoningSplit := true
	if found && extension.ReasoningSplit != nil {
		reasoningSplit = *extension.ReasoningSplit
	}
	extraFields := target.ExtraFields()
	merged := make(map[string]any, len(extraFields)+1)
	for key, value := range extraFields {
		merged[key] = value
	}
	merged[reasoningSplitField] = reasoningSplit
	if found && extension.Thinking != "" {
		merged["thinking"] = map[string]any{"type": extension.Thinking}
	}
	if found && extension.ServiceTier != "" {
		merged["service_tier"] = extension.ServiceTier
	}
	target.SetExtraFields(merged)
	return nil
}

type combinedRequestDialect struct {
	reasoning openaiRequestDialect
}

type openaiRequestDialect interface {
	PrepareRequest(source *corechat.Request, target *openaisdk.ChatCompletionNewParams) error
}

func (dialect combinedRequestDialect) PrepareRequest(source *corechat.Request, target *openaisdk.ChatCompletionNewParams) error {
	if dialect.reasoning == nil {
		return errors.New("minimax: reasoning request dialect is required")
	}
	if err := dialect.reasoning.PrepareRequest(source, target); err != nil {
		return err
	}
	return reasoningSplitDialect{}.PrepareRequest(source, target)
}
