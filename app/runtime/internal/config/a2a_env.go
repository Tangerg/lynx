package config

import (
	"fmt"
	"strings"
)

// parseA2AAgents parses the LYRA_A2A_AGENTS env var: a comma-separated list of
// "name=cardURL" pairs, where cardURL is the base URL the remote agent's
// AgentCard is resolved from. Empty input yields nil. The name becomes the
// delegation tool's name; the first '=' separates it from the URL, so query
// strings in the URL are preserved.
func parseA2AAgents(raw string) ([]A2AAgentConfig, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]A2AAgentConfig, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eq := strings.IndexByte(p, '=')
		if eq <= 0 || eq == len(p)-1 {
			return nil, fmt.Errorf("entry %q: expected name=cardURL", p)
		}
		name := strings.TrimSpace(p[:eq])
		url := strings.TrimSpace(p[eq+1:])
		if name == "" || url == "" {
			return nil, fmt.Errorf("entry %q: name and cardURL must be non-empty", p)
		}
		out = append(out, A2AAgentConfig{Name: name, CardURL: url})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
