package chatclient

import (
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
)

func prepareRequest(request *chat.Request, defaults chat.Options) (*chat.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	prepared := request.Clone()
	prepared.Options = mergeOptions(defaults, request.Options)
	if err := prepared.Validate(); err != nil {
		return nil, err
	}
	return prepared, nil
}

func mergeOptions(defaults, overrides chat.Options) chat.Options {
	merged := defaults.Clone()
	overrides = overrides.Clone()
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
	if overrides.Stop != nil {
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
