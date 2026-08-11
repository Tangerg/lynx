package agentexec

import (
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

const agentDocPromptMaxBytes = 32 * 1024

// agentDocumentsPrompt formats discovered files for the agent system prompt. The
// provenance marker and byte budget are part of the model-facing prompt, not
// the agent-document domain value.
type agentDocumentsPrompt struct {
	text    string
	sources contextSources
}

func newAgentDocumentsPrompt(files []workspace.AgentDocFile, maxBytes int) agentDocumentsPrompt {
	if len(files) == 0 || maxBytes <= 0 {
		return agentDocumentsPrompt{}
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
		return agentDocumentsPrompt{}
	}

	var prompt strings.Builder
	prompt.Grow(total)
	for i := start; i < len(files); i++ {
		if i > start {
			prompt.WriteByte('\n')
		}
		prompt.WriteString(blocks[i])
	}
	sources := make(contextSources, 0, len(files)-start)
	for _, file := range files[start:] {
		sources = append(sources, contextSourceAgentDocument.source(file.Path))
	}
	return agentDocumentsPrompt{text: prompt.String(), sources: sources}
}

func (prompt agentDocumentsPrompt) appendTo(composition *promptComposition) {
	if prompt.text == "" {
		return
	}
	composition.append(
		"## Project context (from AGENTS.md cascade)\n\n"+prompt.text,
		prompt.sources[0],
		prompt.sources[1:]...,
	)
}
