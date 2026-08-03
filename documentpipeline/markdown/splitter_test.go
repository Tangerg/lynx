package markdown_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/documentpipeline"
	markdownsplitter "github.com/Tangerg/lynx/documentpipeline/markdown"
)

type runeTokenizer struct{}

func (runeTokenizer) Encode(ctx context.Context, text string) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tokens := make([]int, 0, len(text))
	for _, value := range text {
		tokens = append(tokens, int(value))
	}
	return tokens, nil
}

func (runeTokenizer) Decode(ctx context.Context, tokens []int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	values := make([]rune, len(tokens))
	for index, token := range tokens {
		values[index] = rune(token)
	}
	return string(values), nil
}

func TestSplitterRepeatsHeadingPathAndTableHeader(t *testing.T) {
	const (
		heading = "# Catalog"
		header  = "| Name | Price |\n| --- | --- |"
		source  = heading + "\n\n" + header + "\n| Alpha | 1 |\n| Beta | 2 |\n| Gamma | 3 |"
	)
	limit := len([]rune(heading + "\n\n" + header + "\n| Alpha | 1 |"))
	splitter := newSplitter(t, limit, 0)

	chunks, err := splitter.SplitText(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("chunk count = %d, want 3: %#v", len(chunks), chunks)
	}
	for index, chunk := range chunks {
		if !strings.HasPrefix(chunk, heading+"\n\n"+header) {
			t.Fatalf("chunk %d lacks heading path or table header: %q", index, chunk)
		}
		assertWithinLimit(t, chunk, limit)
	}
}

func TestSplitterKeepsListItemsWhole(t *testing.T) {
	const (
		heading = "# Tasks"
		first   = "- alpha item"
		second  = "- beta item"
		third   = "- gamma item"
		source  = heading + "\n\n" + first + "\n" + second + "\n" + third
	)
	limit := len([]rune(heading + "\n\n" + first))
	splitter := newSplitter(t, limit, 0)

	chunks, err := splitter.SplitText(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("chunk count = %d, want 3: %#v", len(chunks), chunks)
	}
	for _, want := range []string{first, second, third} {
		matched := false
		for _, chunk := range chunks {
			if strings.Contains(chunk, want) {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("list item %q was lost or severed: %#v", want, chunks)
		}
	}
}

func TestSplitterRepeatsFencesAroundCodeLines(t *testing.T) {
	const (
		heading = "# Example"
		opening = "```go"
		closing = "```"
		source  = heading + "\n\n" + opening + "\nfirst()\nsecond()\nthird()\n" + closing
	)
	limit := len([]rune(heading + "\n\n" + opening + "\nsecond()\n" + closing))
	splitter := newSplitter(t, limit, 0)

	chunks, err := splitter.SplitText(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("chunk count = %d, want 3: %#v", len(chunks), chunks)
	}
	for index, chunk := range chunks {
		if strings.Count(chunk, opening) != 1 || !strings.HasSuffix(chunk, closing) {
			t.Fatalf("chunk %d has invalid code fences: %q", index, chunk)
		}
		assertWithinLimit(t, chunk, limit)
	}
}

func TestSplitterUsesFullHeadingAncestry(t *testing.T) {
	const source = "# Guide\n\nintro\n\n## Install\n\nfirst paragraph\n\nsecond paragraph"
	limit := len([]rune("# Guide\n\n## Install\n\nsecond paragraph"))
	splitter := newSplitter(t, limit, 0)

	chunks, err := splitter.SplitText(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	last := chunks[len(chunks)-1]
	if !strings.HasPrefix(last, "# Guide\n\n## Install\n\n") {
		t.Fatalf("nested heading ancestry missing: %q", last)
	}
}

func TestSplitterFallsBackToTokenWindowsForLongParagraph(t *testing.T) {
	const source = "abcdefghijklmnopqrstuvwxyz"
	splitter := newSplitter(t, 7, 0)

	chunks, err := splitter.SplitText(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 4 {
		t.Fatalf("chunk count = %d, want 4: %#v", len(chunks), chunks)
	}
	for _, chunk := range chunks {
		assertWithinLimit(t, chunk, 7)
	}
}

func TestSplitterRejectsIndivisibleSemanticUnit(t *testing.T) {
	const source = "| Name | Value |\n| --- | --- |\n| impossibly-long-row | value |"
	splitter := newSplitter(t, 25, 0)

	_, err := splitter.SplitText(t.Context(), source)
	if !errors.Is(err, markdownsplitter.ErrSemanticUnitTooLarge) {
		t.Fatalf("SplitText() error = %v, want ErrSemanticUnitTooLarge", err)
	}
	if !strings.Contains(err.Error(), "table row requires") {
		t.Fatalf("error does not identify failing semantic unit: %v", err)
	}
}

func TestSplitterEnforcesChunkLimit(t *testing.T) {
	splitter := newSplitter(t, 3, 2)
	_, err := splitter.SplitText(t.Context(), "abcdefghij")
	if !errors.Is(err, documentpipeline.ErrChunkLimitExceeded) {
		t.Fatalf("SplitText() error = %v, want ErrChunkLimitExceeded", err)
	}
}

func TestTransformPreservesLineageAndAssignsIDs(t *testing.T) {
	splitter, err := markdownsplitter.NewSplitter(markdownsplitter.SplitterConfig{
		Tokenizer:         runeTokenizer{},
		MaxTokensPerChunk: 4,
		IDGenerator:       documentpipeline.NewSHA256IDGenerator(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := document.NewDocument("abcdefgh", nil)
	if err != nil {
		t.Fatal(err)
	}
	doc.ID = "source"
	if err := doc.Metadata.Set("tenant", "one"); err != nil {
		t.Fatal(err)
	}

	chunks, err := splitter.Transform(t.Context(), []*document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(chunks))
	}
	for index, chunk := range chunks {
		if chunk.ID == "" {
			t.Fatalf("chunk %d has no generated ID", index)
		}
		parent, ok, err := metadata.Decode[string](chunk.Metadata, documentpipeline.MetadataKeyParentID)
		if err != nil || !ok || parent != "source" {
			t.Fatalf("chunk %d parent = %q, %v, %v", index, parent, ok, err)
		}
	}
}

func TestSplitterValidatesConfigurationAndCancellation(t *testing.T) {
	var typedNil *nilTokenizer
	for _, config := range []markdownsplitter.SplitterConfig{
		{},
		{Tokenizer: typedNil},
		{Tokenizer: runeTokenizer{}, MaxTokensPerChunk: -1},
		{Tokenizer: runeTokenizer{}, MaxChunks: -1},
	} {
		if _, err := markdownsplitter.NewSplitter(config); err == nil {
			t.Fatalf("NewSplitter(%+v) error = nil", config)
		}
	}

	splitter := newSplitter(t, 10, 0)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := splitter.SplitText(ctx, "body"); !errors.Is(err, context.Canceled) {
		t.Fatalf("SplitText() error = %v, want context.Canceled", err)
	}
}

type nilTokenizer struct{}

func (*nilTokenizer) Encode(context.Context, string) ([]int, error) { return nil, nil }
func (*nilTokenizer) Decode(context.Context, []int) (string, error) { return "", nil }

func newSplitter(t *testing.T, maxTokens, maxChunks int) *markdownsplitter.Splitter {
	t.Helper()
	splitter, err := markdownsplitter.NewSplitter(markdownsplitter.SplitterConfig{
		Tokenizer:         runeTokenizer{},
		MaxTokensPerChunk: maxTokens,
		MaxChunks:         maxChunks,
	})
	if err != nil {
		t.Fatal(err)
	}
	return splitter
}

func assertWithinLimit(t *testing.T, chunk string, limit int) {
	t.Helper()
	if count := len([]rune(chunk)); count > limit {
		t.Fatalf("chunk has %d tokens, maximum is %d: %q", count, limit, chunk)
	}
}
