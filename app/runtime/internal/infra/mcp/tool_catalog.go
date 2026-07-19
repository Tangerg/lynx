package mcp

import (
	"fmt"

	"github.com/Tangerg/lynx/tools"
)

// validateToolCatalog rejects model-facing name collisions across live MCP
// servers. The public label is sanitized and capped by provider constraints, so
// distinct raw identities can collapse to one name; allowing both through would
// make the next agent turn fail registry construction and leave model-facing
// dispatch ambiguous.
//
// replacing is excluded from the current catalog for reconnect/configure. At
// boot it is nil because the candidate has not joined servers yet. The caller
// serializes access to servers.
func validateToolCatalog(servers []*server, replacing *server, candidateServer string, candidate []tools.Tool) error {
	owners := make(map[string]string)
	for _, current := range servers {
		if current == replacing || current.session == nil {
			continue
		}
		for _, tool := range current.tools {
			owners[tool.Definition().Name] = current.name()
		}
	}
	for _, tool := range candidate {
		name := tool.Definition().Name
		if owner, collision := owners[name]; collision {
			return fmt.Errorf(
				"mcp: public tool name collision %q between servers %q and %q",
				name,
				owner,
				candidateServer,
			)
		}
		owners[name] = candidateServer
	}
	return nil
}
