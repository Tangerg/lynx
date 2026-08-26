package agentexec

import (
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

const agentDocPromptMaxBytes = 32 * 1024

const agentDocPromptHeader = "## Project context (from AGENTS.md cascade)"

// agentDocumentsPrompt formats discovered files for the agent system prompt. The
// provenance marker and byte budget are part of the model-facing prompt, not
// the agent-document domain value.
type agentDocumentsPrompt struct {
	text    string
	sources contextSources
}

func newAgentDocumentsPrompt(files []workspace.AgentDocFile, maxBytes int) (agentDocumentsPrompt, error) {
	if len(files) == 0 || maxBytes <= 0 {
		return agentDocumentsPrompt{}, nil
	}
	if err := workspace.ValidateAgentDocumentCascade(files); err != nil {
		return agentDocumentsPrompt{}, err
	}

	blocks := make([]string, len(files))
	sizes := make([]int, len(files))
	total := len(agentDocPromptHeader) + 2
	for i, file := range files {
		blocks[i] = "<!-- From: " + file.Path + " -->\n" + file.Content + "\n"
		sizes[i] = len(blocks[i])
		if len(agentDocPromptHeader)+2+sizes[i] > maxBytes {
			return agentDocumentsPrompt{}, fmt.Errorf(
				"%w: agent document %q cannot fit the %d-byte Run guidance budget",
				workspace.ErrPromptSourceTooLarge,
				file.Path,
				maxBytes,
			)
		}
		total += sizes[i]
	}
	if len(files) > 1 {
		total += len(files) - 1
	}

	start := 0
	for start < len(files) && total > maxBytes {
		total -= sizes[start]
		if start+1 < len(files) {
			total--
		}
		start++
	}
	if start == len(files) {
		return agentDocumentsPrompt{}, nil
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
	return agentDocumentsPrompt{text: prompt.String(), sources: sources}, nil
}

func (a agentDocumentsPrompt) appendTo(composition *promptComposition) {
	if a.text == "" {
		return
	}
	composition.append(
		agentDocPromptHeader+"\n\n"+a.text,
		a.sources[0],
		a.sources[1:]...,
	)
}
