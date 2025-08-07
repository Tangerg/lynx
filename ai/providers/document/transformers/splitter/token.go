package splitter

import (
	"context"
	"errors"
	"strings"

	"github.com/pkoukk/tiktoken-go"

	"github.com/Tangerg/lynx/ai/content/document"
)

type tokenSplitter struct {
	encoding              *tiktoken.Tiktoken
	chunkSize             int
	minChunkSizeChars     int
	minChunkLengthToEmbed int
	maxNumChunks          int
	keepSeparator         bool
}

func (t *tokenSplitter) encodeTokens(text string) []int {
	return t.encoding.Encode(text, nil, nil)
}

func (t *tokenSplitter) decodeTokens(tokens []int) string {
	return t.encoding.Decode(tokens)
}

func (t *tokenSplitter) split(text string) []string {
	if strings.TrimSpace(text) == "" {
		return []string{}
	}

	tokens := t.encodeTokens(text)
	chunks := make([]string, 0, t.chunkSize)
	chunkCount := 0

	for len(tokens) > 0 && chunkCount < t.maxNumChunks {
		chunkEnd := min(t.chunkSize, len(tokens))
		chunk := tokens[:chunkEnd]
		chunkText := t.decodeTokens(chunk)

		if strings.TrimSpace(chunkText) == "" {
			tokens = tokens[len(chunk):]
			continue
		}

		lastPunctuation := max(
			strings.LastIndex(chunkText, "."),
			max(strings.LastIndex(chunkText, "?"),
				max(strings.LastIndex(chunkText, "!"),
					strings.LastIndex(chunkText, "\n"))),
		)

		if lastPunctuation != -1 && lastPunctuation > t.minChunkSizeChars {
			chunkText = chunkText[:lastPunctuation+1]
		}

		var processedChunk string
		if t.keepSeparator {
			processedChunk = strings.TrimSpace(chunkText)
		} else {
			processedChunk = strings.TrimSpace(
				strings.ReplaceAll(chunkText, "\n", " "),
			)
		}

		if len(processedChunk) > t.minChunkLengthToEmbed {
			chunks = append(chunks, processedChunk)
		}

		processedTokens := t.encodeTokens(chunkText)
		tokens = tokens[len(processedTokens):]

		chunkCount++
	}

	if len(tokens) > 0 {
		remainingText := strings.TrimSpace(
			strings.ReplaceAll(
				t.decodeTokens(tokens),
				"\n",
				" ",
			),
		)

		if len(remainingText) > t.minChunkLengthToEmbed {
			chunks = append(chunks, remainingText)
		}
	}

	return chunks
}

var _ document.Transformer = (*TokenSplitter)(nil)

type TokenSplitter struct {
	splitter *Splitter
}

func (s *TokenSplitter) SetCopyFormatter(copyFormatter bool) *TokenSplitter {
	s.splitter.SetCopyFormatter(copyFormatter)
	return s
}

func (s *TokenSplitter) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	return s.splitter.Transform(ctx, docs)
}

type TokenSplitterBuilder struct {
	encoding              *tiktoken.Tiktoken
	chunkSize             int
	minChunkSizeChars     int
	minChunkLengthToEmbed int
	maxNumChunks          int
	keepSeparator         bool
}

func NewTokenSplitterBuilder() *TokenSplitterBuilder {
	return &TokenSplitterBuilder{
		chunkSize:             800,
		minChunkSizeChars:     350,
		minChunkLengthToEmbed: 5,
		maxNumChunks:          10000,
		keepSeparator:         true,
	}
}

func (b *TokenSplitterBuilder) WithEncoding(encoding *tiktoken.Tiktoken) *TokenSplitterBuilder {
	if encoding != nil {
		b.encoding = encoding
	}
	return b
}

func (b *TokenSplitterBuilder) WithEncodingByEncodingName(encodingName string) *TokenSplitterBuilder {
	if encoding, err := tiktoken.GetEncoding(encodingName); err == nil {
		b.encoding = encoding
	}
	return b
}

func (b *TokenSplitterBuilder) WithChunkSize(chunkSize int) *TokenSplitterBuilder {
	if chunkSize > 0 {
		b.chunkSize = chunkSize
	}
	return b
}

func (b *TokenSplitterBuilder) WithMinChunkSizeChars(minChunkSizeChars int) *TokenSplitterBuilder {
	if minChunkSizeChars > 0 {
		b.minChunkSizeChars = minChunkSizeChars
	}
	return b
}

func (b *TokenSplitterBuilder) WithMinChunkLengthToEmbed(minChunkLengthToEmbed int) *TokenSplitterBuilder {
	if minChunkLengthToEmbed > 0 {
		b.minChunkLengthToEmbed = minChunkLengthToEmbed
	}
	return b
}

func (b *TokenSplitterBuilder) WithMaxNumChunks(maxNumChunks int) *TokenSplitterBuilder {
	if maxNumChunks > 0 {
		b.maxNumChunks = maxNumChunks
	}
	return b
}

func (b *TokenSplitterBuilder) WithKeepSeparator(keepSeparator bool) *TokenSplitterBuilder {
	b.keepSeparator = keepSeparator
	return b
}

func (b *TokenSplitterBuilder) Build() (*TokenSplitter, error) {
	if b.encoding == nil {
		return nil, errors.New("encoding is required")
	}

	splitter := &tokenSplitter{
		encoding:              b.encoding,
		chunkSize:             b.chunkSize,
		minChunkSizeChars:     b.minChunkSizeChars,
		minChunkLengthToEmbed: b.minChunkLengthToEmbed,
		maxNumChunks:          b.maxNumChunks,
		keepSeparator:         b.keepSeparator,
	}

	return &TokenSplitter{
		splitter: NewSplitter(splitter.split),
	}, nil
}
