package lexer

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"unicode"

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

// markTokenStart snapshots the cursor as the start of the next token.
// The column is rolled back by one because [consumeChar] has already
// stepped past the first character of the token.
func (l *Lexer) markTokenStart() {
	l.startPosition = l.cursor
	l.startPosition.Column = max(l.startPosition.Column-1, 1)
}

// emitEOF synthesizes an EOF token at the current position.
func (l *Lexer) emitEOF() token.Token {
	l.markTokenStart()
	return token.OfEOF(l.startPosition)
}

// emitError wraps a low-level read error into an ERROR token.
func (l *Lexer) emitError(err error) token.Token {
	l.markTokenStart()
	return token.OfError(err, l.startPosition)
}

// emitIllegal surfaces an unexpected character as an ILLEGAL token.
func (l *Lexer) emitIllegal() token.Token {
	l.markTokenStart()
	return token.OfIllegal(l.currentChar, l.startPosition)
}

// emitKind builds a fixed-shape token (operators, punctuation,
// keywords) using the start/cursor span.
func (l *Lexer) emitKind(kind token.Kind) token.Token {
	return token.OfKind(kind, l.startPosition, l.cursor)
}

// emitLiteral builds a value-bearing token (NUMBER/STRING) using the
// current span.
func (l *Lexer) emitLiteral(kind token.Kind, literal string) token.Token {
	return token.OfLiteral(kind, literal, l.startPosition, l.cursor)
}

// emitIdent builds an IDENT token using the current span.
func (l *Lexer) emitIdent(literal string) token.Token {
	return token.OfIdent(literal, l.startPosition, l.cursor)
}

// peekNextChar returns the next rune without advancing. Returns
// (0, io.EOF) at end of input. Internally reads-then-unreads so the
// underlying reader's position is preserved.
func (l *Lexer) peekNextChar() (rune, error) {
	next, _, err := l.reader.ReadRune()
	if err != nil {
		return 0, err
	}
	if err = l.reader.UnreadRune(); err != nil {
		return 0, err
	}
	return next, nil
}

// consumeChar advances by one rune, updating line/column. Newlines
// reset the column. Returns io.EOF at end of input.
func (l *Lexer) consumeChar() error {
	char, _, err := l.reader.ReadRune()
	if err != nil {
		return err
	}

	l.currentChar = char

	if char == '\n' {
		l.cursor.Line++
		l.cursor.ResetColumn()
	} else {
		l.cursor.Column++
	}
	return nil
}

// bufferCurrentChar appends the rune just consumed to the value
// buffer. Used during string / number / identifier scanning.
func (l *Lexer) bufferCurrentChar() {
	l.valueBuffer.WriteRune(l.currentChar)
}

// consumeExpected consumes one rune that must match expected. It
// panics on mismatch — the caller is required to peek first, so a
// mismatch indicates a lexer-internal bug, not bad user input.
func (l *Lexer) consumeExpected(expected rune) {
	if err := l.consumeChar(); err != nil {
		panic(fmt.Errorf("lexer.consumeExpected: read %q: %w", expected, err))
	}
	if l.currentChar != expected {
		panic(fmt.Errorf("lexer.consumeExpected: want %q, got %q", expected, l.currentChar))
	}
}

// skipWhitespace advances past any run of unicode whitespace and
// leaves the cursor on the next significant rune.
func (l *Lexer) skipWhitespace() error {
	for {
		if err := l.consumeChar(); err != nil {
			return err
		}
		if !unicode.IsSpace(l.currentChar) {
			return nil
		}
	}
}

// resolveEscape turns the rune following a backslash into its actual
// value. Unknown sequences pass through unchanged — same as Go's
// strconv behaviour for an unrecognised escape inside a quoted
// string.
func (l *Lexer) resolveEscape(char rune) rune {
	switch char {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	case '\'':
		return '\''
	case '\\':
		return '\\'
	default:
		return char
	}
}

// scanString tokenizes a single-quoted string. The opening quote is
// the current char; the closing quote is consumed but not buffered.
// Backslash escapes are honoured.
func (l *Lexer) scanString() token.Token {
	defer l.valueBuffer.Reset()

	for {
		if err := l.consumeChar(); err != nil {
			return l.emitError(err)
		}

		if l.currentChar == '\'' {
			break
		}

		if l.currentChar == '\\' {
			if err := l.consumeChar(); err != nil {
				return l.emitError(err)
			}
			l.valueBuffer.WriteRune(l.resolveEscape(l.currentChar))
			continue
		}

		l.bufferCurrentChar()
	}

	return l.emitLiteral(token.STRING, l.valueBuffer.String())
}

// collectDigits reads consecutive ASCII digits into the buffer.
// Stops on first non-digit (peeked, not consumed) or EOF.
func (l *Lexer) collectDigits() error {
	for {
		next, err := l.peekNextChar()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if !unicode.IsDigit(next) {
			return nil
		}

		l.consumeExpected(next)
		l.bufferCurrentChar()
	}
}

// scanNumber tokenizes an integer or decimal number. The first digit
// must be the current char. A decimal point requires at least one
// digit after it.
func (l *Lexer) scanNumber() token.Token {
	defer l.valueBuffer.Reset()

	l.bufferCurrentChar()

	if err := l.collectDigits(); err != nil {
		return l.emitError(err)
	}

	next, err := l.peekNextChar()
	if err != nil && !errors.Is(err, io.EOF) {
		return l.emitError(err)
	}

	if err == nil && next == '.' {
		l.consumeExpected(next)
		l.bufferCurrentChar()

		if err = l.consumeChar(); err != nil {
			return l.emitError(err)
		}
		if !unicode.IsDigit(l.currentChar) {
			return l.emitIllegal()
		}
		l.bufferCurrentChar()

		if err = l.collectDigits(); err != nil {
			return l.emitError(err)
		}
	}

	return l.emitLiteral(token.NUMBER, l.valueBuffer.String())
}

// scanNegativeNumber tokenizes a number that begins with '-'. The
// minus must be immediately followed by a digit.
func (l *Lexer) scanNegativeNumber() token.Token {
	if err := l.consumeChar(); err != nil {
		return l.emitError(err)
	}
	if !unicode.IsDigit(l.currentChar) {
		return l.emitIllegal()
	}

	number := l.scanNumber()
	if !number.Kind.Is(token.NUMBER) {
		return number
	}
	return l.emitLiteral(token.NUMBER, "-"+number.Literal)
}

// scanIdentifier tokenizes an identifier or a keyword. The first
// letter is the current char. A keyword match returns a canonical
// fixed-kind token; everything else returns IDENT preserving case.
func (l *Lexer) scanIdentifier() token.Token {
	defer l.valueBuffer.Reset()

	for {
		l.bufferCurrentChar()

		next, err := l.peekNextChar()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return l.emitError(err)
		}
		if !token.IsLiteralChar(next) {
			break
		}

		l.consumeExpected(next)
	}

	value := l.valueBuffer.String()
	kind := token.KindOf(value)

	if kind.IsKeyword() {
		return l.emitKind(kind)
	}
	return l.emitIdent(value)
}

// scanOptionalSecondChar handles operators where the second character
// is optional — `<` / `<=`, `>` / `>=`. Peeks one ahead to decide.
func (l *Lexer) scanOptionalSecondChar(secondChar rune, single, paired token.Kind) token.Token {
	next, err := l.peekNextChar()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return l.emitKind(single)
		}
		return l.emitError(err)
	}

	if next != secondChar {
		return l.emitKind(single)
	}

	l.consumeExpected(next)
	return l.emitKind(paired)
}

// scanRequiredSecondChar handles operators where the second character
// is mandatory — `==` / `!=`. Mismatch yields ILLEGAL.
func (l *Lexer) scanRequiredSecondChar(secondChar rune, kind token.Kind) token.Token {
	if err := l.consumeChar(); err != nil {
		return l.emitError(err)
	}
	if l.currentChar != secondChar {
		return l.emitIllegal()
	}
	return l.emitKind(kind)
}

// dispatchToken inspects the current rune and routes to the right
// scanner. The first switch covers fixed punctuation/operators; then
// digits and letters fall through to multi-char scanners.
func (l *Lexer) dispatchToken() token.Token {
	switch l.currentChar {
	case '=':
		return l.scanRequiredSecondChar('=', token.EQ)
	case '!':
		return l.scanRequiredSecondChar('=', token.NE)
	case '<':
		return l.scanOptionalSecondChar('=', token.LT, token.LE)
	case '>':
		return l.scanOptionalSecondChar('=', token.GT, token.GE)
	case '\'':
		return l.scanString()
	case '-':
		return l.scanNegativeNumber()
	case '(':
		return l.emitKind(token.LPAREN)
	case ')':
		return l.emitKind(token.RPAREN)
	case '[':
		return l.emitKind(token.LBRACK)
	case ']':
		return l.emitKind(token.RBRACK)
	case ',':
		return l.emitKind(token.COMMA)
	}

	if unicode.IsDigit(l.currentChar) {
		return l.scanNumber()
	}
	if unicode.IsLetter(l.currentChar) {
		return l.scanIdentifier()
	}

	return l.emitIllegal()
}

// Scan returns the next token. EOF and read errors are surfaced as
// EOF / ERROR tokens — callers never see plain io errors.
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

// Token returns an iterator over [Lexer.Scan]. The iterator yields
// indefinitely — consumers stop by returning false. EOF tokens are
// included so consumers can detect end-of-input themselves.
func (l *Lexer) Token() iter.Seq[token.Token] {
	return func(yield func(token.Token) bool) {
		for {
			if !yield(l.Scan()) {
				return
			}
		}
	}
}

// Tokens consumes the entire input and returns every token, EOF
// included. Convenient for tests and small inputs; for large inputs
// prefer [Lexer.Scan] or [Lexer.Token] to avoid buffering everything.
//
// The capacity hint is sized so most realistic filter expressions
// avoid reslicing.
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

// Reset rewinds to the start of input and clears all per-scan state
// — positions, the current rune, the value buffer. The input string
// itself is preserved so the same instance can be reused.
func (l *Lexer) Reset() {
	l.startPosition.Reset()
	l.cursor.Reset()
	l.currentChar = 0
	l.reader.Reset(l.input)
	l.valueBuffer.Reset()
}
