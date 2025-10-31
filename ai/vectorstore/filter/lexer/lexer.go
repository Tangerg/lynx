package lexer

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"unicode"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// Lexer represents a lexical analyzer that tokenizes input text into a stream of tokens.
// It maintains currentPosition tracking, character buffering, and provides both single-Token
// and batch tokenization capabilities.
type Lexer struct {
	input           string           // The complete source input string being tokenized
	startPosition   token.Position   // Position where the current Token starts
	currentPosition token.Position   // Current reading currentPosition with line and column tracking
	currentChar     rune             // The character currently being examined
	reader          *strings.Reader  // String reader for character-by-character processing
	valueBuffer     *strings.Builder // Reusable buffer for constructing Token literal values
}

// NewLexer creates and initializes a new lexer instance for tokenizing the provided input.
// Returns an error if the input is empty. The lexer is positioned at the beginning
// and ready to start tokenization.
func NewLexer(input string) (*Lexer, error) {
	if len(input) == 0 {
		return nil, errors.New("input cannot be empty")
	}

	return &Lexer{
		input:           input,
		startPosition:   token.NewPosition(),
		currentPosition: token.NewPosition(),
		currentChar:     0,
		reader:          strings.NewReader(input),
		valueBuffer:     &strings.Builder{},
	}, nil
}

// markTokenStart captures the current currentPosition as the start of a new Token.
// Adjusts column backward by one to point to the actual character that
// triggered the Token recognition.
func (l *Lexer) markTokenStart() {
	l.startPosition = l.currentPosition
	l.startPosition.Column = max(l.startPosition.Column-1, 1)
}

// createEOFToken creates an EOF Token with proper position information.
// Standardizes EOF Token creation by ensuring consistent position tracking.
func (l *Lexer) createEOFToken() token.Token {
	l.markTokenStart()
	return token.OfEOF(l.startPosition)
}

// createErrorToken creates an error Token with proper currentPosition information.
// Standardizes error Token creation by ensuring consistent error reporting format.
func (l *Lexer) createErrorToken(err error) token.Token {
	l.markTokenStart()
	return token.OfError(err, l.startPosition)
}

// createIllegalToken creates an illegal character Token with proper currentPosition information.
// Used for characters that don't belong in the language grammar.
func (l *Lexer) createIllegalToken() token.Token {
	l.markTokenStart()
	return token.OfIllegal(l.currentChar, l.startPosition)
}

// createKindToken creates a Token using the kind's default literal value.
// Uses the current lexer currentPosition range from startPosition to current currentPosition.
func (l *Lexer) createKindToken(kind token.Kind) token.Token {
	return token.OfKind(kind, l.startPosition, l.currentPosition)
}

// createLiteralToken creates a Token with custom literal content and validation.
// Applies validation/normalization based on the Token kind, particularly for NUMBER tokens.
func (l *Lexer) createLiteralToken(kind token.Kind, literal string) token.Token {
	return token.OfLiteral(kind, literal, l.startPosition, l.currentPosition)
}

// createIdentToken creates an identifier Token with the given literal value.
// Uses the current lexer currentPosition range from startPosition to current currentPosition.
func (l *Lexer) createIdentToken(literal string) token.Token {
	return token.OfIdent(literal, l.startPosition, l.currentPosition)
}

// peekNextChar examines the next character without consuming it.
// Essential for tokenization decisions where current character interpretation
// depends on what follows (e.g., '<' vs '<='). Returns 0 and io.EOF at end of input.
func (l *Lexer) peekNextChar() (rune, error) {
	nextChar, _, err := l.reader.ReadRune()
	if err != nil {
		return 0, err
	}

	// Rewind to maintain current currentPosition
	if err = l.reader.UnreadRune(); err != nil {
		return 0, err
	}

	return nextChar, nil
}

// consumeChar advances to the next character and updates currentPosition tracking.
// Handles newline detection for accurate line and column reporting.
// Returns io.EOF when end of input is reached.
func (l *Lexer) consumeChar() error {
	char, _, err := l.reader.ReadRune()
	if err != nil {
		return err
	}

	l.currentChar = char

	// Update currentPosition tracking - handle newlines specially
	if char == '\n' {
		l.currentPosition.Line++
		l.currentPosition.ResetColumn()
	} else {
		l.currentPosition.Column++
	}

	return nil
}

// bufferCurrentChar adds the current character to the value buffer.
// Used during Token scanning to accumulate characters for Token value construction.
func (l *Lexer) bufferCurrentChar() {
	l.valueBuffer.WriteRune(l.currentChar)
}

// consumeExpectedChar reads and validates the next character matches expected value.
// Panics if expectation is violated, indicating a programming error in lexer logic.
// Should only be called after successful peekNextChar() confirmation.
func (l *Lexer) consumeExpectedChar(expectedChar rune) {
	if err := l.consumeChar(); err != nil {
		panic(fmt.Errorf("failed to read expected character '%c': %w", expectedChar, err))
	}
	if l.currentChar != expectedChar {
		panic(fmt.Errorf("expected '%c' but found '%c'", expectedChar, l.currentChar))
	}
}

// skipWhitespace advances past all consecutive whitespace characters.
// Positions the lexer at the start of the next meaningful Token.
// Returns io.EOF when end of input is reached while skipping whitespace.
func (l *Lexer) skipWhitespace() error {
	for {
		if err := l.consumeChar(); err != nil {
			return err
		}

		if !unicode.IsSpace(l.currentChar) {
			break
		}
	}
	return nil
}

// escapeChar converts escape sequence characters to their actual representations.
// Transforms escape sequences like 'n', 't', 'r' into '\n', '\t', '\r'.
// Used during string literal parsing to interpret escape sequences.
func (l *Lexer) escapeChar(char rune) rune {
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
		return char // Unknown escape sequences treated as literal characters
	}
}

// scanStringLiteral tokenizes a string enclosed in single quotes with escape support.
// Handles character accumulation, escape sequence processing, and error handling
// for unterminated strings. Opening quote should already be current character.
func (l *Lexer) scanStringLiteral() token.Token {
	defer l.valueBuffer.Reset()

	for {
		if err := l.consumeChar(); err != nil {
			return l.createErrorToken(err)
		}

		// Found closing quote - string literal complete
		if l.currentChar == '\'' {
			break
		}

		// Handle escape sequences
		if l.currentChar == '\\' {
			// Consume the character following the backslash
			if err := l.consumeChar(); err != nil {
				return l.createErrorToken(err)
			}

			escapedChar := l.escapeChar(l.currentChar)
			l.valueBuffer.WriteRune(escapedChar)
		} else {
			l.bufferCurrentChar()
		}
	}

	return l.createLiteralToken(token.STRING, l.valueBuffer.String())
}

// collectDigits reads consecutive digit characters into the value buffer.
// Stops when a non-digit character is encountered or EOF is reached.
// Uses peekNextChar to avoid consuming the terminating character.
func (l *Lexer) collectDigits() error {
	for {
		nextChar, err := l.peekNextChar()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil // End of input is valid termination
			}
			return err
		}

		if !unicode.IsDigit(nextChar) {
			break
		}

		l.consumeExpectedChar(nextChar)
		l.bufferCurrentChar()
	}
	return nil
}

// scanNumericLiteral tokenizes integer and floating-point number literals.
// Handles both simple integers (123) and decimal numbers (123.45) with
// proper validation. First digit should already be current character.
func (l *Lexer) scanNumericLiteral() token.Token {
	defer l.valueBuffer.Reset()

	// Add the initial digit
	l.bufferCurrentChar()

	// Collect remaining integer digits
	if err := l.collectDigits(); err != nil {
		return l.createErrorToken(err)
	}

	// Check for decimal point
	nextChar, err := l.peekNextChar()
	if err != nil && !errors.Is(err, io.EOF) {
		return l.createErrorToken(err)
	}

	// Process fractional part if decimal point present
	if err == nil && nextChar == '.' {
		l.consumeExpectedChar(nextChar)
		l.bufferCurrentChar()

		// Decimal point must be followed by at least one digit
		if err = l.consumeChar(); err != nil {
			return l.createErrorToken(err)
		}

		if !unicode.IsDigit(l.currentChar) {
			return l.createIllegalToken()
		}

		l.bufferCurrentChar()

		// Collect remaining fractional digits
		if err = l.collectDigits(); err != nil {
			return l.createErrorToken(err)
		}
	}

	return l.createLiteralToken(token.NUMBER, l.valueBuffer.String())
}

// scanNegativeNumericLiteral tokenizes negative numeric literals starting with minus sign.
// Validates that minus is immediately followed by digits and constructs negative number Token.
// Minus sign should already be current character.
func (l *Lexer) scanNegativeNumericLiteral() token.Token {
	// Character after minus must be a digit
	if err := l.consumeChar(); err != nil {
		return l.createErrorToken(err)
	}

	if !unicode.IsDigit(l.currentChar) {
		return l.createIllegalToken()
	}

	// Scan numeric portion
	numericToken := l.scanNumericLiteral()
	if !numericToken.Kind.Is(token.NUMBER) {
		return numericToken
	}

	// Prepend minus sign to create negative literal
	negativeValue := "-" + numericToken.Literal
	return l.createLiteralToken(token.NUMBER, negativeValue)
}

// scanIdentifier tokenizes identifiers and keywords starting with a letter.
// Accumulates valid identifier characters and determines if result is a reserved keyword
// or user-defined identifier. Case-sensitive for identifiers, case-insensitive for keywords.
func (l *Lexer) scanIdentifier() token.Token {
	defer l.valueBuffer.Reset()

	for {
		l.bufferCurrentChar()

		nextChar, err := l.peekNextChar()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return l.createErrorToken(err)
		}

		if !token.IsLiteralChar(nextChar) {
			break
		}

		l.consumeExpectedChar(nextChar)
	}

	identifierValue := l.valueBuffer.String()
	tokenKind := token.KindOf(identifierValue)

	// Use canonical form for keywords, preserve case for identifiers
	if tokenKind.IsKeyword() {
		return l.createKindToken(tokenKind)
	}

	return l.createIdentToken(identifierValue)
}

// scanVariableLengthOperator handles operators that can be one or two characters.
// Examines next character to determine single vs double character operator.
// Examples: '<' vs '<=' and '>' vs '>='.
func (l *Lexer) scanVariableLengthOperator(secondChar rune, singleCharKind, doubleCharKind token.Kind) token.Token {
	nextChar, err := l.peekNextChar()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return l.createKindToken(singleCharKind)
		}
		return l.createErrorToken(err)
	}

	if nextChar != secondChar {
		return l.createKindToken(singleCharKind)
	}

	// Consume second character for two-character operator
	l.consumeExpectedChar(nextChar)
	return l.createKindToken(doubleCharKind)
}

// scanFixedLengthOperator handles operators that must be exactly two characters.
// Enforces specific two-character sequences and creates appropriate operator Token.
// Examples: '==' and '!='. Rejects partial sequences as illegal.
func (l *Lexer) scanFixedLengthOperator(requiredSecondChar rune, operatorKind token.Kind) token.Token {
	if err := l.consumeChar(); err != nil {
		return l.createErrorToken(err)
	}

	if l.currentChar != requiredSecondChar {
		return l.createIllegalToken()
	}

	return l.createKindToken(operatorKind)
}

// dispatchToken analyzes current character and routes to appropriate scanning method.
// Main tokenization dispatcher implementing the lexer's decision tree for determining
// Token types. Handles all supported categories and delegates to specialized scanners.
func (l *Lexer) dispatchToken() token.Token {
	switch l.currentChar {
	case '=':
		return l.scanFixedLengthOperator('=', token.EQ)
	case '!':
		return l.scanFixedLengthOperator('=', token.NE)
	case '<':
		return l.scanVariableLengthOperator('=', token.LT, token.LE)
	case '>':
		return l.scanVariableLengthOperator('=', token.GT, token.GE)
	case '\'':
		return l.scanStringLiteral()
	case '-':
		return l.scanNegativeNumericLiteral()
	case '(':
		return l.createKindToken(token.LPAREN)
	case ')':
		return l.createKindToken(token.RPAREN)
	case '[':
		return l.createKindToken(token.LBRACK)
	case ']':
		return l.createKindToken(token.RBRACK)
	case ',':
		return l.createKindToken(token.COMMA)
	}

	if unicode.IsDigit(l.currentChar) {
		return l.scanNumericLiteral()
	}

	if unicode.IsLetter(l.currentChar) {
		return l.scanIdentifier()
	}

	return l.createIllegalToken()
}

// Scan returns the next Token from the input stream.
// Primary method for single-Token extraction. Handles whitespace skipping,
// end-of-input detection, and delegates to dispatchToken for recognition.
func (l *Lexer) Scan() token.Token {
	// Skip leading whitespace
	if err := l.skipWhitespace(); err != nil {
		if errors.Is(err, io.EOF) {
			return l.createEOFToken()
		}
		return l.createErrorToken(err)
	}

	l.markTokenStart()
	return l.dispatchToken()
}

// Token returns an iterator that yields tokens one by one from the input stream.
// This method provides a convenient way to iterate over all tokens using Go's
// The iterator continues until the consumer stops requesting tokens or
// an error occurs. Each call to the iterator internally calls Scan().
func (l *Lexer) Token() iter.Seq[token.Token] {
	return func(yield func(token.Token) bool) {
		for {
			if !yield(l.Scan()) {
				return
			}
		}
	}
}

// Tokens processes entire input and returns all tokens as a slice.
// Convenience method for batch processing. Continues until EOF token encountered.
// Includes ERROR and ILLEGAL tokens in result for error recovery and debugging.
//
// Note: This method consumes the entire input stream and stores all tokens in memory.
// For large inputs, consider using Scan() or Token() for streaming processing.
func (l *Lexer) Tokens() []token.Token {
	var allTokens []token.Token
	for currentToken := range l.Token() {
		allTokens = append(allTokens, currentToken)

		if currentToken.Kind.Is(token.EOF) {
			break
		}
	}

	return allTokens
}

// Reset reinitializes the lexer to begin tokenizing from start of input.
// Clears all internal state including positions, buffers, and reader currentPosition.
// Enables efficient reuse of lexer instances for multiple parsing operations.
func (l *Lexer) Reset() {
	l.startPosition.Reset()
	l.currentPosition.Reset()
	l.currentChar = 0
	l.reader.Reset(l.input)
	l.valueBuffer.Reset()
}
