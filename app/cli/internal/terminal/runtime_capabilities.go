package terminal

import (
	"fmt"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/runtimeprofile"
)

// runtimeSupports is optimistic only for backends without discovery, such as
// the scripted demo runtime. A discovered runtime is authoritative: a missing,
// disabled, or declined opt-in feature is unavailable.
func (a *app) runtimeSupports(feature runtimeprofile.FeatureName) bool {
	return a.runtimeProfile == nil || a.runtimeProfile.Supports(feature)
}

func (a *app) requireRuntimeFeature(feature runtimeprofile.FeatureName) error {
	if a.runtimeSupports(feature) {
		return nil
	}
	return fmt.Errorf("runtime capability %q was not negotiated", feature)
}

func (a *app) validateMessageCapabilities(message agent.Message) error {
	for _, attachment := range message.Attachments {
		if attachment.Kind == agent.AttachmentImage {
			return a.requireRuntimeFeature(runtimeprofile.FeatureMultimodal)
		}
	}
	return nil
}

func availableWithRuntimeFeature(a *app, feature runtimeprofile.FeatureName) CommandAvailability {
	if err := a.requireRuntimeFeature(feature); err != nil {
		return CommandAvailability{Reason: err.Error()}
	}
	return CommandAvailability{Enabled: true}
}
