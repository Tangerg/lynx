package agentmemory

import (
	"slices"
	"strings"
)

const charsPerToken = 4

// EstimateTokens approximates a token count for a memory budget: one token per
// non-ASCII rune (CJK and friends tokenize roughly per-character) plus one per
// [charsPerToken] ASCII bytes. Deliberately cheap and provider-agnostic.
func EstimateTokens(text string) int {
	ascii := 0
	tokens := 0
	for _, r := range text {
		if r <= 0x7f {
			ascii++
		} else {
			tokens++
		}
	}
	return tokens + (ascii+charsPerToken-1)/charsPerToken
}

// Render assembles the memory body injected into the system prompt: pinned
// items first, then the rest by recency, accumulated until maxTokens would be
// exceeded. The budget is a defensive whole-inject bound for the always-on
// core; retrieval trims the wider corpus separately. Returns "" when there is
// nothing to inject. maxTokens <= 0 means unbounded.
func Render(items []Item, maxTokens int) string {
	if len(items) == 0 {
		return ""
	}
	ordered := slices.Clone(items)
	slices.SortStableFunc(ordered, func(a, b Item) int {
		if a.Pinned != b.Pinned {
			if a.Pinned {
				return -1
			}
			return 1
		}
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	var b strings.Builder
	used := 0
	for _, item := range ordered {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		cost := EstimateTokens(content)
		if maxTokens > 0 && b.Len() > 0 && used+cost > maxTokens {
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(content)
		used += cost
	}
	return b.String()
}
