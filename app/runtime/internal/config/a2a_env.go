package config

import (
	"fmt"
	"slices"
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
	seen := make(map[string]struct{}, len(parts))
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
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("entry %q: agent %q is configured more than once", p, name)
		}
		seen[name] = struct{}{}
		out = append(out, A2AAgentConfig{Name: name, CardURL: url})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// addA2ARPCOrigins applies the optional LYRA_A2A_RPC_ORIGINS map to parsed
// agents. The shape is "name=origin|origin,name=origin"; names must already
// exist in LYRA_A2A_AGENTS so a misspelling cannot silently weaken nothing.
func addA2ARPCOrigins(agents []A2AAgentConfig, raw string) ([]A2AAgentConfig, error) {
	if raw == "" {
		return agents, nil
	}
	out := make([]A2AAgentConfig, len(agents))
	for i, agent := range agents {
		out[i] = agent
		out[i].AllowedRPCOrigins = slices.Clone(agent.AllowedRPCOrigins)
	}
	byName := make(map[string]int, len(agents))
	for i, agent := range out {
		if _, duplicate := byName[agent.Name]; duplicate {
			return nil, fmt.Errorf("agent %q is configured more than once", agent.Name)
		}
		byName[agent.Name] = i
	}
	configured := make(map[string]struct{})
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, rawOrigins, ok := strings.Cut(entry, "=")
		name, rawOrigins = strings.TrimSpace(name), strings.TrimSpace(rawOrigins)
		if !ok || name == "" || rawOrigins == "" {
			return nil, fmt.Errorf("entry %q: expected name=origin|origin", entry)
		}
		index, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("entry %q: agent %q is not configured", entry, name)
		}
		if _, duplicate := configured[name]; duplicate {
			return nil, fmt.Errorf("entry %q: agent %q is configured more than once", entry, name)
		}
		origins := strings.Split(rawOrigins, "|")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
			if origins[i] == "" {
				return nil, fmt.Errorf("entry %q: RPC origins must not be empty", entry)
			}
		}
		out[index].AllowedRPCOrigins = origins
		configured[name] = struct{}{}
	}
	return out, nil
}
