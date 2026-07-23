package agentexec

import (
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentdoc"
)

const agentDocPromptMaxBytes = 32 * 1024

// renderAgentDocs formats discovered files for the agent system prompt. The
// provenance marker and byte budget are part of the model-facing prompt, not
// the agent-document domain value.
func renderAgentDocs(files []agentdoc.File, maxBytes int) string {
	if len(files) == 0 || maxBytes <= 0 {
		return ""
	}

	blocks := make([]string, len(files))
	sizes := make([]int, len(files))
	total := 0
	for i, file := range files {
		blocks[i] = "<!-- From: " + file.Path + " -->\n" + file.Content + "\n"
		sizes[i] = len(blocks[i])
		total += sizes[i]
	}
	if len(files) > 1 {
		total += len(files) - 1
	}

	start := 0
	for start < len(files) && total > maxBytes {
		total -= sizes[start]
		if start > 0 {
			total--
		}
		start++
	}
	if start == len(files) {
		return ""
	}

	var prompt strings.Builder
	prompt.Grow(total)
	for i := start; i < len(files); i++ {
		if i > start {
			prompt.WriteByte('\n')
		}
		prompt.WriteString(blocks[i])
	}
	return prompt.String()
}
