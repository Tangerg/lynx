package documentpipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/documentpipeline"
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

func TestTokenSplitterHonorsTokenAndSentenceBounds(t *testing.T) {
	splitter, err := documentpipeline.NewTokenSplitter(documentpipeline.TokenSplitterConfig{
		Tokenizer:             runeTokenizer{},
		MaxTokensPerChunk:     10,
		MinTokensPerChunk:     4,
		MinCharactersPerChunk: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := document.NewDocument("abcdef.ghijklmnop", nil)
	chunks, err := splitter.Split(t.Context(), []*document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(chunks))
	}
	if chunks[0].Text != "abcdef." {
		t.Fatalf("first chunk = %q, want sentence boundary", chunks[0].Text)
	}
	for index, chunk := range chunks {
		if len([]rune(chunk.Text)) > 10 {
			t.Fatalf("chunk %d has %d tokens, want at most 10", index, len([]rune(chunk.Text)))
		}
	}
}

func TestTokenSplitterFailsInsteadOfEmittingOversizedTail(t *testing.T) {
	splitter, err := documentpipeline.NewTokenSplitter(documentpipeline.TokenSplitterConfig{
		Tokenizer:             runeTokenizer{},
		MaxTokensPerChunk:     3,
		MinTokensPerChunk:     1,
		MinCharactersPerChunk: 1,
		MaxChunks:             1,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := document.NewDocument("abcdefgh", nil)
	if _, err := splitter.Split(t.Context(), []*document.Document{doc}); !errors.Is(err, documentpipeline.ErrChunkLimitExceeded) {
		t.Fatalf("Split() error = %v, want ErrChunkLimitExceeded", err)
	}
}

func TestTokenSplitterValidatesLimits(t *testing.T) {
	for _, config := range []documentpipeline.TokenSplitterConfig{
		{},
		{Tokenizer: runeTokenizer{}, MaxTokensPerChunk: -1},
		{Tokenizer: runeTokenizer{}, MaxTokensPerChunk: 2, MinTokensPerChunk: 3},
	} {
		if _, err := documentpipeline.NewTokenSplitter(config); err == nil {
			t.Fatalf("NewTokenSplitter(%+v) error = nil", config)
		}
	}
}
