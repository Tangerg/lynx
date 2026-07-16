package event

import "github.com/Tangerg/lynx/agent/core"

// AgentDeployed fires when an agent is registered on a Engine.
type AgentDeployed struct {
	Header
	Deployment core.DeploymentRef `json:"deployment"`
}

func (AgentDeployed) Kind() string { return "agent_deployed" }

// AgentUndeployed fires when an agent is removed from a Engine.
type AgentUndeployed struct {
	Header
	Deployment core.DeploymentRef `json:"deployment"`
}

func (AgentUndeployed) Kind() string { return "agent_undeployed" }
