package chat

import (
	"regexp"
	"strings"
)

// ThinkingTagCleaner removes thinking-tag wrappers commonly emitted by
// reasoning-style models when their reasoning output is exposed inline in
// the response text rather than via a structured field. Different models
// use different wrapper conventions; the default cleaner recognizes:
//
//   - <thinking>...</thinking>   (Amazon Nova)
//   - <think>...</think>         (Qwen, DeepSeek-R1 distill, …)
//   - <reasoning>...</reasoning>
//   - ```thinking ... ```        (Markdown fenced)
//   - <!-- thinking: ... -->     (HTML comment)
//
// This is the Go counterpart to Spring AI's ThinkingTagCleaner and exists
// for the same reason: when a model bakes its reasoning into the visible
// text instead of exposing a separate reasoning_content field, downstream
// structured parsers (JSON, schema-typed) blow up on the thinking prefix.
// The fast path keeps the cleaner cheap enough to call on every parse.
//
// Patterns are matched case-insensitively and across newlines (regex (?is)
// flags). Cleaning is deterministic: each pattern is applied in turn until
// the string stops shrinking; nested wrappers are handled by the
// non-greedy quantifier within each pattern.
type ThinkingTagCleaner struct {
	patterns []*regexp.Regexp
}

// DefaultThinkingTagPatterns lists the wrapper regexes used by NewThinkingTagCleaner.
// The slice is exported so callers can extend or replace it explicitly.
var DefaultThinkingTagPatterns = []string{
	`(?is)<thinking>.*?</thinking>\s*`,
	`(?is)<think>.*?</think>\s*`,
	`(?is)<reasoning>.*?</reasoning>\s*`,
	"(?is)```thinking.*?```\\s*",
	`(?is)<!--\s*thinking:.*?-->\s*`,
}

// NewThinkingTagCleaner returns a cleaner configured with the default
// pattern set. Use NewThinkingTagCleanerWithPatterns to supply a custom
// list of regex sources.
func NewThinkingTagCleaner() *ThinkingTagCleaner {
	return mustNewThinkingTagCleaner(DefaultThinkingTagPatterns)
}

// NewThinkingTagCleanerWithPatterns returns a cleaner that strips text
// matching any of the supplied regex sources. Each pattern is compiled
// eagerly; a malformed pattern triggers a panic, mirroring how Go's
// regexp.MustCompile handles invalid input. Pass an empty slice to disable
// cleaning (Clean becomes a no-op fast-path identity function).
func NewThinkingTagCleanerWithPatterns(patterns []string) *ThinkingTagCleaner {
	return mustNewThinkingTagCleaner(patterns)
}

func mustNewThinkingTagCleaner(patterns []string) *ThinkingTagCleaner {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(p))
	}
	return &ThinkingTagCleaner{patterns: compiled}
}

// Clean removes any thinking-tag wrappers from input. Empty inputs and
// inputs that obviously cannot contain a wrapper short-circuit without
// touching any regex engine, so the cleaner is safe to apply
// unconditionally before structured parsing. Cleaning is non-destructive:
// no surrounding whitespace beyond the trailing-space suffix in each
// pattern is consumed.
func (c *ThinkingTagCleaner) Clean(input string) string {
	if input == "" || len(c.patterns) == 0 {
		return input
	}
	// Fast path: if the input contains neither '<' nor '`', no default
	// pattern can match. Custom patterns that bypass these markers will
	// still run because we fall through whenever either character appears.
	if !strings.ContainsAny(input, "<`") {
		return input
	}
	result := input
	for _, p := range c.patterns {
		result = p.ReplaceAllString(result, "")
	}
	return result
}

// defaultThinkingTagCleaner is shared by structured parsers and other
// callers that don't need a custom configuration. It is safe for
// concurrent use because regexp.Regexp is read-only after compilation.
var defaultThinkingTagCleaner = NewThinkingTagCleaner()

// CleanThinkingTags is a package-level convenience that delegates to a
// shared default cleaner. Use it from hot paths where allocating a
// per-call cleaner would be wasteful.
func CleanThinkingTags(input string) string {
	return defaultThinkingTagCleaner.Clean(input)
}
