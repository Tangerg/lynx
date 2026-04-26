package text

import "testing"

func TestThinkingTagCleaner_Default_FastPath(t *testing.T) {
	c := NewThinkingTagCleaner()

	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"plain text", "hello world"},
		{"json without tags", `{"foo":"bar","n":1}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.Clean(tc.in)
			if got != tc.in {
				t.Fatalf("expected fast-path identity, got %q for %q", got, tc.in)
			}
		})
	}
}

func TestThinkingTagCleaner_Default_StripsKnownPatterns(t *testing.T) {
	c := NewThinkingTagCleaner()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "qwen think tag",
			in:   "<think>I am thinking…</think>{\"answer\":42}",
			want: "{\"answer\":42}",
		},
		{
			name: "amazon nova thinking tag",
			in:   "<thinking>let me reason</thinking>final",
			want: "final",
		},
		{
			name: "reasoning tag case insensitive",
			in:   "<REASONING>x</Reasoning>tail",
			want: "tail",
		},
		{
			name: "markdown thinking fence",
			in:   "```thinking\nstep 1\nstep 2\n```{\"a\":1}",
			want: "{\"a\":1}",
		},
		{
			name: "html comment thinking",
			in:   "<!-- thinking: hidden -->visible",
			want: "visible",
		},
		{
			name: "multiline non greedy",
			in:   "<think>a</think>middle<think>b</think>end",
			want: "middleend",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.Clean(tc.in)
			if got != tc.want {
				t.Fatalf("Clean(%q): want %q, got %q", tc.in, tc.want, got)
			}
		})
	}
}

func TestCleanThinkingTags_PackageLevel(t *testing.T) {
	const in = "<think>hidden reasoning</think>visible"
	const want = "visible"
	if got := CleanThinkingTags(in); got != want {
		t.Fatalf("CleanThinkingTags: want %q, got %q", want, got)
	}
}

func TestThinkingTagCleaner_CustomPatterns(t *testing.T) {
	c := NewThinkingTagCleanerWithPatterns([]string{`(?is)<scratchpad>.*?</scratchpad>\s*`})
	const in = "<scratchpad>x</scratchpad>kept"
	if got := c.Clean(in); got != "kept" {
		t.Fatalf("custom pattern: want kept, got %q", got)
	}
	// Default <think> tag must NOT be stripped because we replaced patterns.
	const passthrough = "<think>not stripped</think>tail"
	if got := c.Clean(passthrough); got != passthrough {
		t.Fatalf("default pattern leaked into custom cleaner: %q", got)
	}
}

func TestThinkingTagCleaner_EmptyPatterns_NoOp(t *testing.T) {
	c := NewThinkingTagCleanerWithPatterns(nil)
	const in = "<think>kept</think>tail"
	if got := c.Clean(in); got != in {
		t.Fatalf("empty pattern set should be a no-op, got %q", got)
	}
}
