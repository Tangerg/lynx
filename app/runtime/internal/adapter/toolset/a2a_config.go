package toolset

import (
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
)

// A2AAgentConfig identifies one remote Agent-to-Agent endpoint to expose as a
// delegation tool in the assembled tool environment.
type A2AAgentConfig struct {
	Name              string
	CardURL           string
	AllowedRPCOrigins []string
}

func infraA2AClientConfigs(in []A2AAgentConfig) []a2a.ClientConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]a2a.ClientConfig, len(in))
	for i, agent := range in {
		out[i] = a2a.ClientConfig{
			Name:              agent.Name,
			CardURL:           agent.CardURL,
			AllowedRPCOrigins: slices.Clone(agent.AllowedRPCOrigins),
		}
	}
	return out
}
