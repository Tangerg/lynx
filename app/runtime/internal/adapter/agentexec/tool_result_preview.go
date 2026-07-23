package agentexec

import (
	"fmt"
	"unicode/utf8"
)

// renderToolResultPreview renders the model-facing replacement for an offloaded
// tool result. This wording contains the agent tool's retrieval contract, so it
// belongs with the executor adapter rather than in a generic component.
func renderToolResultPreview(body, id, readToolName string, previewBytes int) string {
	previewBytes = min(max(previewBytes, 0), len(body))
	head := previewBytes * 3 / 4
	tailStart := len(body) - previewBytes/4
	for head > 0 && !utf8.RuneStart(body[head]) {
		head--
	}
	for tailStart < len(body) && !utf8.RuneStart(body[tailStart]) {
		tailStart++
	}
	if tailStart < head {
		tailStart = head
	}
	marker := fmt.Sprintf(
		"\n\n…[%d bytes offloaded to keep the context small — retrieve the full output with the %s tool: {\"id\":\"%s\"} (supports offset/limit).]…\n\n",
		len(body), readToolName, id,
	)
	return body[:head] + marker + body[tailStart:]
}
