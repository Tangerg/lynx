package agentexec

import (
	"slices"
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/domain/agentmemory"
)

const memoryPromptCharsPerToken = 4

// pinnedMemoryPrompt projects memory values into the agent system prompt. Its
// ordering, text form, and token approximation belong to the model adapter;
// the domain retains only memory lifecycle and content invariants.
type pinnedMemoryPrompt struct {
	text    string
	sources contextSources
}

func newPinnedMemoryPrompt(items []agentmemory.Item, maxTokens int) pinnedMemoryPrompt {
	if len(items) == 0 {
		return pinnedMemoryPrompt{}
	}
	ordered := slices.Clone(items)
	slices.SortStableFunc(ordered, func(a, b agentmemory.Item) int {
		if a.Pinned != b.Pinned {
			if a.Pinned {
				return -1
			}
			return 1
		}
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})

	var prompt strings.Builder
	sources := make(contextSources, 0, len(ordered))
	used := 0
	for _, item := range ordered {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		cost := estimateMemoryPromptTokens(content)
		if maxTokens > 0 && used+cost > maxTokens {
			break
		}
		if prompt.Len() > 0 {
			prompt.WriteByte('\n')
		}
		prompt.WriteString(content)
		sources = append(sources, contextSourcePinnedMemory.source(item.ID))
		used += cost
	}
	return pinnedMemoryPrompt{text: prompt.String(), sources: sources}
}

func (p pinnedMemoryPrompt) appendTo(composition *promptComposition) {
	if p.text == "" {
		return
	}
	composition.append(
		"## Pinned memory (managed by ScopeApp)\n\n"+p.text,
		p.sources[0],
		p.sources[1:]...,
	)
}

func estimateMemoryPromptTokens(text string) int {
	ascii := 0
	tokens := 0
	for _, r := range text {
		if r <= 0x7f {
			ascii++
		} else {
			tokens++
		}
	}
	return tokens + (ascii+memoryPromptCharsPerToken-1)/memoryPromptCharsPerToken
}
