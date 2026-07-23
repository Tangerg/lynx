package agentexec

import (
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

const memoryPromptCharsPerToken = 4

// renderPinnedMemory projects memory values into the agent system prompt. Its
// ordering, text form, and token approximation belong to the model adapter;
// the domain retains only memory lifecycle and content invariants.
func renderPinnedMemory(items []agentmemory.Item, maxTokens int) string {
	if len(items) == 0 {
		return ""
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
	used := 0
	for _, item := range ordered {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		cost := estimateMemoryPromptTokens(content)
		if maxTokens > 0 && prompt.Len() > 0 && used+cost > maxTokens {
			break
		}
		if prompt.Len() > 0 {
			prompt.WriteByte('\n')
		}
		prompt.WriteString(content)
		used += cost
	}
	return prompt.String()
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
