package agentexec

import (
	"fmt"
	"slices"
	"strings"
)

const maxAuthoredGuidanceBytes = 32 << 10

const agentDocumentPreamble = `Lyra workspace guidance follows. Each section is authored by the user or project at the named source. Apply more specific, later sections when guidance conflicts.`

// renderAgentDocuments keeps complete documents and favors the most-specific
// tail of the Runtime's root-to-leaf ordering when the aggregate prompt is too
// large. A single document is never truncated or silently ignored.
func renderAgentDocuments(documents []AgentDocument) (string, error) {
	values := make([]promptDocument, 0, len(documents))
	for index, document := range documents {
		path := strings.TrimSpace(document.Path)
		content := strings.TrimSpace(document.Content)
		if path == "" || content == "" {
			return "", fmt.Errorf("agentexec: agent document %d is incomplete", index)
		}
		values = append(values, promptDocument{kind: "agent", path: path, content: content})
	}
	return renderPromptDocuments(agentDocumentPreamble, values)
}

type promptDocument struct {
	kind    string
	path    string
	content string
}

func renderPromptDocuments(preamble string, documents []promptDocument) (string, error) {
	if len(documents) == 0 {
		return "", nil
	}
	blocks := make([]string, len(documents))
	for index, document := range documents {
		blocks[index] = fmt.Sprintf("<!-- Lyra source: %s -->\n%s", document.path, document.content)
		if len(preamble)+2+len(blocks[index]) > maxAuthoredGuidanceBytes {
			return "", fmt.Errorf(
				"agentexec: %s document %s exceeds the %d-byte Run guidance budget",
				document.kind,
				document.path,
				maxAuthoredGuidanceBytes,
			)
		}
	}
	remaining := maxAuthoredGuidanceBytes - len(preamble) - 2
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
	return preamble + "\n\n" + strings.Join(selected, "\n\n"), nil
}
