package agentexec

import (
	"fmt"
	"slices"
	"strings"
)

const maxAgentDocumentPromptBytes = 32 << 10

const agentDocumentPreamble = `Lyra workspace guidance follows. Each section is authored by the user or project at the named source. Apply more specific, later sections when guidance conflicts.`

// renderAgentDocuments keeps complete documents and favors the most-specific
// tail of the Runtime's root-to-leaf ordering when the aggregate prompt is too
// large. A single document is never truncated or silently ignored.
func renderAgentDocuments(documents []AgentDocument) (string, error) {
	if len(documents) == 0 {
		return "", nil
	}
	blocks := make([]string, len(documents))
	for index, document := range documents {
		path := strings.TrimSpace(document.Path)
		content := strings.TrimSpace(document.Content)
		if path == "" || content == "" {
			return "", fmt.Errorf("agentexec: agent document %d is incomplete", index)
		}
		blocks[index] = fmt.Sprintf("<!-- Lyra source: %s -->\n%s", path, content)
		if len(agentDocumentPreamble)+2+len(blocks[index]) > maxAgentDocumentPromptBytes {
			return "", fmt.Errorf(
				"agentexec: agent document %s exceeds the %d-byte Run guidance budget",
				path,
				maxAgentDocumentPromptBytes,
			)
		}
	}
	remaining := maxAgentDocumentPromptBytes - len(agentDocumentPreamble) - 2
	selected := make([]string, 0, len(blocks))
	for index := len(blocks) - 1; index >= 0; index-- {
		required := len(blocks[index])
		if len(selected) > 0 {
			required += 2
		}
		if required > remaining {
			break
		}
		selected = append(selected, blocks[index])
		remaining -= required
	}
	slices.Reverse(selected)
	return agentDocumentPreamble + "\n\n" + strings.Join(selected, "\n\n"), nil
}
