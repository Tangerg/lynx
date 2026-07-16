package event

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

// InteractionBoundary binds one model/tool protocol event to the exact process
// deployment and logical interaction that produced it.
type InteractionBoundary struct {
	Header
	Deployment    core.DeploymentRef `json:"deployment"`
	InteractionID string             `json:"interaction_id"`
	Boundary      interaction.Event  `json:"boundary"`
}

func (InteractionBoundary) Kind() string { return "interaction_boundary" }
