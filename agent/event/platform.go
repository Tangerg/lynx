package event

// AgentDeployedEvent fires when an agent is registered on a Platform.
type AgentDeployedEvent struct {
	BaseEvent
	AgentName string `json:"agent_name"`
}

func (AgentDeployedEvent) EventName() string { return "agent_deployed" }

func (e AgentDeployedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"agent_name": e.AgentName})
}

// AgentUndeployedEvent fires when an agent is removed from a Platform.
type AgentUndeployedEvent struct {
	BaseEvent
	AgentName string `json:"agent_name"`
}

func (AgentUndeployedEvent) EventName() string { return "agent_undeployed" }

func (e AgentUndeployedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"agent_name": e.AgentName})
}
