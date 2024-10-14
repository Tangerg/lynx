package splitter

import (
	"github.com/Tangerg/lynx/ai/core/tokenizer"
	pkgSystem "github.com/Tangerg/lynx/pkg/system"
	"strings"
)

type TokenSplitter struct {
	TextSplitter
	tokenizer             tokenizer.Tokenizer
	defaultChunkSize      int
	minChunkSizeChars     int
	minChunkLengthToEmbed int
	maxNumChunks          int
	keepSeparator         bool
}

func (t *TokenSplitter) splitText(text string) []string {
	return t.doSplit(text, t.defaultChunkSize)
}

func (t *TokenSplitter) doSplit(text string, chunkSize int) []string {
	if strings.TrimSpace(text) == "" {
		return []string{}
	}
	tokens := t.tokenizer.EncodeTokens(text)
	chunks := make([]string, 0, chunkSize)
	if len(tokens) == 0 {
		return chunks
	}
	for range t.maxNumChunks {
		if len(tokens) == 0 {
			break
		}
		chunk := tokens[0:min(chunkSize, len(tokens))]
		chunkText := t.tokenizer.DecodeTokens(chunk)
		if strings.TrimSpace(chunkText) == "" {
			tokens = tokens[len(chunk):]
			continue
		}
		lastPunctuation := max(
			strings.Index(chunkText, "."),
			strings.Index(chunkText, "?"),
			max(strings.Index(chunkText, "!"),
				strings.Index(chunkText, "\n"),
			),
		)
		if lastPunctuation != -1 &&
			lastPunctuation > t.minChunkSizeChars {
			chunkText = chunkText[:lastPunctuation+1]
		}
		var chunkTextToAppend string
		if t.keepSeparator {
			chunkTextToAppend = strings.TrimSpace(chunkText)
		} else {
			chunkTextToAppend = strings.TrimSpace(
				strings.ReplaceAll(chunkText, pkgSystem.LineSeparator(), " "),
			)
		}
		if len(chunkTextToAppend) > t.minChunkLengthToEmbed {
			chunks = append(chunks, chunkTextToAppend)
		}
		tokens = tokens[len(t.tokenizer.EncodeTokens(chunkText)):]
	}

	if len(tokens) > 0 {
		remainingText := strings.TrimSpace(
			strings.ReplaceAll(
				t.tokenizer.DecodeTokens(tokens),
				pkgSystem.LineSeparator(),
				" ",
			),
		)
		if len(remainingText) > t.minChunkLengthToEmbed {
			chunks = append(chunks, remainingText)
		}
	}

	return chunks
}

func NewTokenSplitterBuilder(tokenizer tokenizer.Tokenizer) *TokenSplitterBuilder {
	ts := &TokenSplitter{
		tokenizer:             tokenizer,
		defaultChunkSize:      800,
		minChunkSizeChars:     350,
		minChunkLengthToEmbed: 5,
		maxNumChunks:          10000,
		keepSeparator:         true,
	}
	ts.TextSplitFunc = ts.splitText
	return &TokenSplitterBuilder{
		ts: ts,
	}
}

type TokenSplitterBuilder struct {
	ts *TokenSplitter
}

func (t *TokenSplitterBuilder) WithDefaultChunkSize(chunkSize int) *TokenSplitterBuilder {
	t.ts.defaultChunkSize = chunkSize
	return t
}
func (t *TokenSplitterBuilder) WithMinChunkSize(chunkSize int) *TokenSplitterBuilder {
	t.ts.minChunkSizeChars = chunkSize
	return t
}
func (t *TokenSplitterBuilder) WithMinChunkLength(chunkLength int) *TokenSplitterBuilder {
	t.ts.minChunkLengthToEmbed = chunkLength
	return t
}
func (t *TokenSplitterBuilder) WithMaxNumChunks(maxNumChunks int) *TokenSplitterBuilder {
	t.ts.maxNumChunks = maxNumChunks
	return t
}
func (t *TokenSplitterBuilder) WithKeepSeparator(keepSeparator bool) *TokenSplitterBuilder {
	t.ts.keepSeparator = keepSeparator
	return t
}
func (t *TokenSplitterBuilder) Build() *TokenSplitter {
	return t.ts
}
