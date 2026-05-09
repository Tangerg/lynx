package event

// AgentDeployed fires when an agent is registered on a Platform.
type AgentDeployed struct {
	BaseEvent
	AgentName string `json:"agent_name"`
}

func (AgentDeployed) EventName() string { return "agent_deployed" }

func (e AgentDeployed) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"agent_name": e.AgentName})
}

// AgentUndeployed fires when an agent is removed from a Platform.
type AgentUndeployed struct {
	BaseEvent
	AgentName string `json:"agent_name"`
}

func (AgentUndeployed) EventName() string { return "agent_undeployed" }

func (e AgentUndeployed) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"agent_name": e.AgentName})
}
