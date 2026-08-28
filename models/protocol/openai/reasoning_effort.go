package openai

import (
	"fmt"

	openaisdk "github.com/openai/openai-go/v3"

	corechat "github.com/Tangerg/scope/core/chat"
)

func mapReasoningEffort(effort corechat.ReasoningEffort) (openaisdk.ReasoningEffort, error) {
	switch effort {
	case "":
		return "", nil
	case corechat.ReasoningEffort(openaisdk.ReasoningEffortNone):
		return openaisdk.ReasoningEffortNone, nil
	case corechat.ReasoningEffort(openaisdk.ReasoningEffortMinimal):
		return openaisdk.ReasoningEffortMinimal, nil
	case corechat.ReasoningEffort(openaisdk.ReasoningEffortLow):
		return openaisdk.ReasoningEffortLow, nil
	case corechat.ReasoningEffort(openaisdk.ReasoningEffortMedium):
		return openaisdk.ReasoningEffortMedium, nil
	case corechat.ReasoningEffort(openaisdk.ReasoningEffortHigh):
		return openaisdk.ReasoningEffortHigh, nil
	case corechat.ReasoningEffort(openaisdk.ReasoningEffortXhigh):
		return openaisdk.ReasoningEffortXhigh, nil
	case corechat.ReasoningEffort(openaisdk.ReasoningEffortMax):
		return openaisdk.ReasoningEffortMax, nil
	default:
		return "", fmt.Errorf("openai: options.reasoning_effort has unsupported value %q", effort)
	}
}
