package lexer

import (
	"errors"
	"io"
	"iter"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// Lexer scans a single input string. It tracks the start position of
// the in-flight token plus the running cursor position, reuses one
// [strings.Builder] for value accumulation, and exposes both
// per-token and iterator/batch APIs.
type Lexer struct {
	input         string
	startPosition token.Position
	cursor        token.Position
	currentChar   rune
	reader        *strings.Reader
	valueBuffer   *strings.Builder
}

// NewLexer creates a [Lexer] positioned at the start of input. Empty
// input is rejected — callers asking to parse nothing have a bug.
func NewLexer(input string) (*Lexer, error) {
	if len(input) == 0 {
		return nil, errors.New("lexer.NewLexer: input must not be empty")
	}

	return &Lexer{
		input:         input,
		startPosition: token.NewPosition(),
		cursor:        token.NewPosition(),
		reader:        strings.NewReader(input),
		valueBuffer:   &strings.Builder{},
	}, nil
}

func (l *Lexer) Scan() token.Token {
	if err := l.skipWhitespace(); err != nil {
		if errors.Is(err, io.EOF) {
			return l.emitEOF()
		}
		return l.emitError(err)
	}

	l.markTokenStart()
	return l.dispatchToken()
}

func (l *Lexer) Token() iter.Seq[token.Token] {
	return func(yield func(token.Token) bool) {
		for {
			if !yield(l.Scan()) {
				return
			}
		}
	}
}

func (l *Lexer) Tokens() []token.Token {
	tokens := make([]token.Token, 0, len(l.input)/4+8)
	for tk := range l.Token() {
		tokens = append(tokens, tk)
		if tk.Kind.Is(token.EOF) {
			break
		}
	}
	return tokens
}

func (l *Lexer) Reset() {
	l.startPosition.Reset()
	l.cursor.Reset()
	l.currentChar = 0
	l.reader.Reset(l.input)
	l.valueBuffer.Reset()
}
