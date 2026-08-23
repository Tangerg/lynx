package agentexec

import (
	"fmt"
	"strings"
)

const knowledgePreamble = `Lyra user and workspace knowledge follows. The sections are ordered from broad preferences to the most specific workspace knowledge; later sections take precedence when they conflict.`

func renderKnowledgeDocuments(documents []KnowledgeDocument) (string, error) {
	values := make([]promptDocument, 0, len(documents))
	for index, document := range documents {
		path := strings.TrimSpace(document.Path)
		content := strings.TrimSpace(document.Content)
		if path == "" || content == "" {
			return "", fmt.Errorf("agentexec: knowledge document %d is incomplete", index)
		}
		values = append(values, promptDocument{
			kind: "knowledge", path: path, content: content,
		})
	}
	return renderPromptDocuments(knowledgePreamble, values)
}
