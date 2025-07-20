// Package filter provides lexical analysis functionality for parsing filter expressions.
// This file implements a lexer that tokenizes input text into a stream of tokens
// for subsequent parsing by a filter expression parser.
package filter

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// Lexer represents a lexical analyzer that tokenizes input text into a stream of tokens.
// It maintains the current reading state including position tracking for accurate error reporting,
// character buffering for efficient processing, and provides both single-token and batch
// tokenization capabilities. The lexer handles various token types including identifiers,
// numbers, strings, operators, and punctuation while properly managing whitespace and
// providing detailed error information for invalid input.
type Lexer struct {
	input         string          // The complete source input string being tokenized
	startPosition token.Position  // Position where the current token starts
	position      token.Position  // Current reading position in input with line and column tracking
	currentChar   rune            // The character currently being examined by the lexer
	reader        *strings.Reader // Efficient string reader for character-by-character processing
	valueBuffer   strings.Builder // Reusable buffer for efficiently constructing token literal values
}

// NewLexer creates and initializes a new lexer instance for tokenizing the provided input.
// The lexer is positioned at the beginning of the input and ready to start tokenization.
//
// Parameters:
//   - input: The source text to be tokenized. Must be non-empty.
//
// Returns:
//   - *Lexer: A fully initialized lexer ready for tokenization
//   - error: An error if the input is empty or invalid
//
// The returned lexer maintains internal state for position tracking and provides
// methods for both single-token scanning and complete tokenization of the input.
func NewLexer(input string) (*Lexer, error) {
	if len(input) == 0 {
		return nil, errors.New("input cannot be empty")
	}

	return &Lexer{
		input:         input,
		startPosition: token.NewPosition(),
		position:      token.NewPosition(),
		currentChar:   0,
		reader:        strings.NewReader(input),
		valueBuffer:   strings.Builder{},
	}, nil
}

// markTokenStart captures the current position as the start of a new token.
// This method is called before token scanning begins to establish the token's
// starting position. The column is adjusted backward by one to point to the
// actual character that triggered the token recognition.
//
// Position adjustment logic:
//   - Sets startPosition to current position
//   - Adjusts column backward by 1 to point to current character
//   - Ensures column never goes below 1 (handles edge cases)
//
// This method should be called immediately after whitespace skipping and
// before dispatching to specific token scanning methods.
func (l *Lexer) markTokenStart() {
	l.startPosition = l.position
	l.startPosition.Column = max(l.startPosition.Column-1, 1)
}

// createErrorToken creates an error token with proper position information.
// This helper method standardizes error token creation by ensuring the start
// position is properly set and providing consistent error reporting format.
//
// Parameters:
//   - err: The error that occurred during tokenization
//
// Returns:
//   - token.Token: An ERROR token with the error message and proper position range
//
// Position handling:
//   - Updates startPosition to point to where the error occurred
//   - Uses the error position as both start and end (errors are point events)
//   - Ensures consistent error reporting across all lexer methods
func (l *Lexer) createErrorToken(err error) token.Token {
	l.markTokenStart()
	return token.OfError(err, l.startPosition)
}

// createIllegalToken creates an illegal character token with proper position information.
// This specialized error token factory handles the common case of encountering
// characters that don't belong in the language grammar.
//
// Returns:
//   - token.Token: An ERROR token describing the illegal character and its location
//
// Token properties:
//   - Contains descriptive error message with character and position
//   - Start position points to the illegal character
//   - End position is NoPosition (illegal characters are point events)
//   - Provides precise location information for debugging
func (l *Lexer) createIllegalToken() token.Token {
	l.markTokenStart()
	return token.OfIllegal(l.currentChar, l.startPosition)
}

// createKindToken creates a token using the kind's default literal value.
// Uses the current lexer position range (from startPosition to current position)
// and automatically derives the literal from the token kind.
//
// Parameters:
//   - kind: The token kind to create
//
// Returns:
//   - token.Token: A token with the kind's default literal and current position range
func (l *Lexer) createKindToken(kind token.Kind) token.Token {
	return token.OfKind(kind, l.startPosition, l.position)
}

// createLiteralToken creates a token with custom literal content and validation.
// Uses the current lexer position range and applies validation/normalization
// based on the token kind (particularly for NUMBER tokens).
//
// Parameters:
//   - kind: The token kind to create
//   - literal: The literal string content for the token
//
// Returns:
//   - token.Token: A validated token, or an error token if validation fails
func (l *Lexer) createLiteralToken(kind token.Kind, literal string) token.Token {
	return token.OfLiteral(kind, literal, l.startPosition, l.position)
}

func (l *Lexer) createIdentToken(literal string) token.Token {
	return token.OfIdent(literal, l.startPosition, l.position)
}

// peekNextChar examines the next character in the input stream without consuming it.
// This lookahead functionality is essential for tokenization decisions where the
// current character's interpretation depends on what follows it (e.g., distinguishing
// between '<' and '<=' operators, or determining if a '-' is part of a negative number).
//
// Returns:
//   - rune: The next character in the stream, or 0 if EOF
//   - error: io.EOF when end of input is reached, or other read errors
//
// The reader position remains unchanged after this call, allowing the next consumeChar()
// call to read the same character that was peeked. This method is safe to call multiple
// times consecutively and will return the same character each time.
func (l *Lexer) peekNextChar() (rune, error) {
	nextChar, _, err := l.reader.ReadRune()
	if err != nil {
		return 0, err
	}

	// Rewind to maintain current position - this ensures peek doesn't consume
	if err = l.reader.UnreadRune(); err != nil {
		return 0, err
	}

	return nextChar, nil
}

// consumeChar advances the lexer to the next character in the input and updates
// position tracking information. This is the primary method for advancing through
// the input stream during tokenization. It automatically handles newline detection
// for accurate line and column reporting in error messages and token positions.
//
// Returns:
//   - error: io.EOF when end of input is reached, or other read errors
//
// Position tracking behavior:
//   - For '\n' characters: increments line counter and resets column to 1
//   - For all other characters: increments column counter
//   - Updates currentChar field with the newly read character
//
// This method should be called whenever the lexer needs to advance to process
// the next character in the tokenization process.
func (l *Lexer) consumeChar() error {
	char, _, err := l.reader.ReadRune()
	if err != nil {
		return err
	}

	l.currentChar = char

	// Update position tracking - handle newlines specially for accurate reporting
	if char == '\n' {
		l.position.Line++
		l.position.ResetColumn()
	} else {
		l.position.Column++
	}

	return nil
}

// consumeExpectedChar reads the next character and validates it matches the expected value.
// This method is used in scenarios where the lexer has already determined (via peekNextChar)
// that a specific character should be present. It provides fail-fast behavior by panicking
// if the expectation is violated, which indicates a programming error in the lexer logic.
//
// Parameters:
//   - expectedChar: The character that must be at the current reader position
//
// Panics:
//   - If the read operation fails (indicating an unexpected I/O error)
//   - If the actual character doesn't match expectedChar (programming error)
//
// This method should only be called after a successful peekNextChar() operation
// has confirmed the character exists and matches expectations. The panic behavior
// is intentional as misuse indicates a bug in the lexer implementation rather than
// invalid user input.
func (l *Lexer) consumeExpectedChar(expectedChar rune) {
	if err := l.consumeChar(); err != nil {
		panic(fmt.Sprintf("failed to read expected character '%c': %v", expectedChar, err))
	}
	if l.currentChar != expectedChar {
		panic(fmt.Sprintf("expected '%c' but found '%c'", expectedChar, l.currentChar))
	}
}

// skipWhitespace advances the lexer past all consecutive whitespace characters.
// This method is called at the beginning of token scanning to position the lexer
// at the start of the next meaningful token. It handles all Unicode whitespace
// characters as defined by the unicode.IsSpace() function.
//
// Returns:
//   - error: io.EOF when end of input is reached, or other read errors
//   - nil: when a non-whitespace character is encountered and becomes currentChar
//
// After successful completion, currentChar will contain the first non-whitespace
// character encountered, and the lexer will be positioned to begin tokenizing
// that character. If EOF is reached while skipping whitespace, io.EOF is returned
// and the caller should generate an EOF token.
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

// escapeChar converts escape sequence characters to their actual special character representations.
// This method transforms escape sequence characters (like 'n', 't', 'r') back into their
// corresponding special characters ('\n', '\t', '\r'). It's used during string literal
// parsing to properly interpret escape sequences within quoted strings.
//
// Parameters:
//   - char: The escape sequence character following a backslash
//
// Returns:
//   - rune: The actual special character represented by the escape sequence
//
// Supported escape mappings:
//   - 'n' -> '\n' (newline)
//   - 't' -> '\t' (tab)
//   - 'r' -> '\r' (carriage return)
//   - '\” -> '\” (single quote)
//   - '\\' -> '\\' (backslash)
//   - All other characters -> unchanged (literal character after backslash)
//
// Usage context:
//
//	This method is called when a backslash is encountered in a string literal,
//	and the following character needs to be interpreted as an escape sequence.
//	For example, in the string 'hello\nworld', when '\' is found, the next
//	character 'n' is passed to this method, which returns '\n'.
//
// Usage examples:
//   - escapeChar('n') returns '\n'
//   - escapeChar('t') returns '\t'
//   - escapeChar('a') returns 'a' (unknown escape, treat as literal)
//   - escapeChar('\”) returns '\”
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
		return char // Unknown escape sequences are treated as literal characters
	}
}

// scanStringLiteral tokenizes a string literal enclosed in single quotes with escape sequence support.
// This method handles the complete process of string tokenization including character accumulation,
// escape sequence processing, and proper error handling for unterminated strings. The opening
// quote should already be the current character when this method is called.
//
// Returns:
//   - token.Token: A STRING token containing the processed literal content, or ERROR token on failure
//
// String literal rules:
//   - Must be enclosed in single quotes ('...')
//   - Supports escape sequences with backslash (\n, \t, \r, \', \\, \0)
//   - Content between quotes becomes the token's literal value after escape processing
//   - Closing quote is required (unterminated strings generate errors)
//   - Empty strings are valid (”)
//
// Escape sequence processing:
//   - Backslash (\) followed by escape character becomes special character
//   - Supported sequences: \n (newline), \t (tab), \r (carriage return)
//   - Quote escaping: \' becomes literal single quote within string
//   - Backslash escaping: \\ becomes literal backslash
//   - Unknown escape sequences treated as literal characters
//
// Processing logic:
//  1. Consume characters until closing quote or EOF
//  2. For regular characters: add directly to buffer
//  3. For backslash: consume next character and apply escape transformation
//  4. Continue until closing quote found
//  5. Return STRING token with processed content
//
// Error conditions:
//   - EOF reached before closing quote (unterminated string)
//   - EOF reached immediately after backslash (incomplete escape sequence)
//   - Read errors during character consumption
//
// The valueBuffer is used for efficient string accumulation and is automatically
// reset after token creation to prepare for the next token.
//
// Example processing:
//
//	Input: 'hello\nworld\'s test\\'
//	Output: STRING token with literal "hello\nworld's test\"
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
				// EOF after backslash - incomplete escape sequence
				return l.createErrorToken(err)
			}

			// Transform escape sequence and add to buffer
			escapedChar := l.escapeChar(l.currentChar)
			l.valueBuffer.WriteRune(escapedChar)
		} else {
			// Regular character - add directly to buffer
			l.valueBuffer.WriteRune(l.currentChar)
		}
	}

	return l.createLiteralToken(token.STRING, l.valueBuffer.String())
}

// collectDigits reads consecutive digit characters into the value buffer.
// This helper method is used by numeric literal scanning to efficiently
// gather digit sequences while handling end-of-input conditions gracefully.
// It stops when a non-digit character is encountered or EOF is reached.
//
// Returns:
//   - error: Read errors other than EOF (EOF is handled as normal termination)
//   - nil: When digit collection completes successfully
//
// Behavior:
//   - Continues reading while characters are digits (unicode.IsDigit)
//   - Stops at first non-digit character without consuming it
//   - EOF during digit collection is treated as normal termination
//   - All collected digits are written to the valueBuffer
//
// This method uses peekNextChar() to avoid consuming the terminating character,
// allowing the caller to properly handle whatever follows the digit sequence.
func (l *Lexer) collectDigits() error {
	for {
		nextChar, err := l.peekNextChar()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil // End of input is valid termination for digit sequences
			}
			return err
		}

		if !unicode.IsDigit(nextChar) {
			break // Non-digit character ends digit collection
		}

		l.consumeExpectedChar(nextChar)
		l.valueBuffer.WriteRune(l.currentChar)
	}
	return nil
}

// scanNumericLiteral tokenizes integer and floating-point number literals.
// This method handles both simple integers (123) and decimal numbers (123.45)
// with proper validation of numeric format. The first digit should already
// be the current character when this method is called.
//
// Returns:
//   - token.Token: A NUMBER token with the complete numeric literal, or ERROR token on failure
//
// Supported formats:
//   - Integers: 123, 0, 999999
//   - Decimals: 123.45, 0.5, 999.001
//   - Invalid: 123., .123, 12.34.56 (multiple decimal points)
//
// Parsing logic:
//  1. Add the initial digit to buffer
//  2. Collect remaining integer digits
//  3. Check for decimal point
//  4. If decimal point found, validate fractional part format
//  5. Collect fractional digits
//
// Error conditions:
//   - I/O errors during character reading
//   - Decimal point not followed by digit (123.)
//   - Read errors in fractional part processing
func (l *Lexer) scanNumericLiteral() token.Token {
	defer l.valueBuffer.Reset()

	// Add the initial digit that triggered this numeric scan
	l.valueBuffer.WriteRune(l.currentChar)

	// Collect remaining digits in the integer portion
	if err := l.collectDigits(); err != nil {
		return l.createErrorToken(err)
	}

	// Check for decimal point to identify floating-point numbers
	nextChar, err := l.peekNextChar()
	if err != nil && !errors.Is(err, io.EOF) {
		return l.createErrorToken(err)
	}

	// Process fractional part if decimal point is present
	if err == nil && nextChar == '.' {
		// Consume the decimal point
		l.consumeExpectedChar(nextChar)
		l.valueBuffer.WriteRune(l.currentChar)

		// Decimal point must be followed by at least one digit (reject "123.")
		if err = l.consumeChar(); err != nil {
			return l.createErrorToken(err)
		}

		if !unicode.IsDigit(l.currentChar) {
			return l.createIllegalToken()
		}

		// Add the first fractional digit
		l.valueBuffer.WriteRune(l.currentChar)

		// Collect any remaining fractional digits
		if err = l.collectDigits(); err != nil {
			return l.createErrorToken(err)
		}
	}

	return l.createLiteralToken(token.NUMBER, l.valueBuffer.String())
}

// scanNegativeNumericLiteral tokenizes negative numeric literals that start with a minus sign.
// This method validates that the minus sign is followed by valid numeric content and
// constructs a negative number token. The minus sign should already be the current character.
//
// Returns:
//   - token.Token: A NUMBER token with negative value, or ERROR/ILLEGAL token on failure
//
// Validation rules:
//   - Minus sign must be immediately followed by a digit
//   - No whitespace allowed between minus and digits
//   - Follows same numeric format rules as positive numbers for fractional part
//
// Processing steps:
//  1. Consume character after minus (must be digit)
//  2. Validate the character is a digit
//  3. Use scanNumericLiteral() to process the numeric portion
//  4. Prepend minus sign to the resulting numeric literal
//
// Error conditions:
//   - EOF immediately after minus sign
//   - Non-digit character following minus (creates ILLEGAL token)
//   - Any errors from numeric portion scanning
func (l *Lexer) scanNegativeNumericLiteral() token.Token {
	// Character immediately following minus must be a digit
	if err := l.consumeChar(); err != nil {
		return l.createErrorToken(err)
	}

	if !unicode.IsDigit(l.currentChar) {
		return l.createIllegalToken()
	}

	// Scan the numeric portion using existing logic
	numericToken := l.scanNumericLiteral()
	if !numericToken.Kind.Is(token.NUMBER) {
		return numericToken
	}

	// Prepend minus sign to create the complete negative literal
	negativeValue := "-" + numericToken.Literal
	return l.createLiteralToken(token.NUMBER, negativeValue)
}

// scanIdentifier tokenizes identifiers and keyword literals that start with a letter.
// This method accumulates valid identifier characters and determines whether the
// result is a reserved keyword or a user-defined identifier. Processing continues
// while characters satisfy the identifier character rules.
//
// Returns:
//   - token.Token: IDENT token for identifiers, or specific keyword token for reserved words
//
// Identifier rules:
//   - Must start with a letter (unicode.IsLetter)
//   - Can contain letters, digits, and underscores
//   - Determined by token.IsLiteralChar() function
//   - Case-sensitive for identifiers, case-insensitive for keyword recognition
//
// Processing logic:
//  1. Add current character (letter) to buffer
//  2. Continue collecting valid identifier characters
//  3. Stop at first non-identifier character (without consuming it)
//  4. Determine if result matches a reserved keyword
//  5. Return appropriate token type with literal value
//
// Keyword handling:
//   - Keywords use canonical lowercase form as literal
//   - Identifiers preserve original case in literal
//   - Keyword recognition is case-insensitive
func (l *Lexer) scanIdentifier() token.Token {
	defer l.valueBuffer.Reset()

	for {
		l.valueBuffer.WriteRune(l.currentChar)

		nextChar, err := l.peekNextChar()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break // End of input terminates identifier normally
			}
			return l.createErrorToken(err)
		}

		// Stop collecting if next character cannot be part of identifier
		if !token.IsLiteralChar(nextChar) {
			break
		}

		l.consumeExpectedChar(nextChar)
	}

	identifierValue := l.valueBuffer.String()
	tokenKind := token.KindOf(identifierValue)

	// Use canonical form for keywords, preserve original case for identifiers
	if tokenKind.IsKeyword() {
		return l.createKindToken(tokenKind)
	}

	return l.createIdentToken(identifierValue)
}

// scanVariableLengthOperator handles operators that can be either one or two characters long.
// This method examines the next character to determine whether to create a single-character
// or double-character operator token. Examples include '<' vs '<=' and '>' vs '>='.
//
// Parameters:
//   - secondChar: The character that would complete the two-character operator
//   - singleCharKind: Token kind for the single-character operator
//   - doubleCharKind: Token kind for the two-character operator
//
// Returns:
//   - token.Token: The appropriate operator token based on the character sequence found
//
// Decision logic:
//   - Peek at next character without consuming it
//   - If next character matches secondChar, consume it and return doubleCharKind token
//   - If next character differs or EOF reached, return singleCharKind token
//   - Handle I/O errors appropriately during lookahead
//
// This method is essential for correctly tokenizing comparison operators where
// single and double character variants have different meanings in the language.
func (l *Lexer) scanVariableLengthOperator(secondChar rune, singleCharKind, doubleCharKind token.Kind) token.Token {
	nextChar, err := l.peekNextChar()
	if err != nil {
		if errors.Is(err, io.EOF) {
			// End of input - return single character operator
			return l.createKindToken(singleCharKind)
		}
		return l.createErrorToken(err)
	}

	// Check if we have the two-character operator variant
	if nextChar != secondChar {
		return l.createKindToken(singleCharKind)
	}

	// Consume second character to complete two-character operator
	l.consumeExpectedChar(nextChar)
	return l.createKindToken(doubleCharKind)
}

// scanFixedLengthOperator handles operators that must be exactly two characters.
// This method enforces that specific two-character sequences are present and
// creates the appropriate operator token. Examples include '==' and '!='.
//
// Parameters:
//   - requiredSecondChar: The character that must follow the current character
//   - operatorKind: The token kind to create if the sequence is valid
//
// Returns:
//   - token.Token: The operator token if sequence is valid, or ERROR/ILLEGAL token on failure
//
// Validation process:
//  1. Consume the next character (must exist)
//  2. Verify it exactly matches requiredSecondChar
//  3. Create operator token if match succeeds
//  4. Create illegal character token if match fails
//
// Error conditions:
//   - EOF encountered when second character expected
//   - Second character doesn't match required value
//   - I/O errors during character consumption
//
// This strict validation ensures that partial operator sequences (like '!' without '=')
// are properly rejected rather than being misinterpreted as valid tokens.
func (l *Lexer) scanFixedLengthOperator(requiredSecondChar rune, operatorKind token.Kind) token.Token {
	if err := l.consumeChar(); err != nil {
		return l.createErrorToken(err)
	}

	// Second character must match exactly for valid operator
	if l.currentChar != requiredSecondChar {
		return l.createIllegalToken()
	}

	return l.createKindToken(operatorKind)
}

// dispatchToken analyzes the current character and routes to the appropriate token scanning method.
// This is the main tokenization dispatcher that implements the lexer's decision tree for
// determining token types based on the first character of each token. It handles all
// supported token categories and delegates to specialized scanning methods.
//
// Returns:
//   - token.Token: The next complete token from the input stream
//
// Token type dispatch logic:
//   - '=' -> Fixed-length operator scan for '=='
//   - '!' -> Fixed-length operator scan for '!='
//   - '<' -> Variable-length operator scan for '<' or '<='
//   - '>' -> Variable-length operator scan for '>' or '>='
//   - "'" -> String literal scan
//   - '-' -> Negative numeric literal scan
//   - '(', ')', '[', ']', ',' -> Single-character punctuation tokens
//   - Digits -> Numeric literal scan
//   - Letters -> Identifier/keyword scan
//   - Other -> Illegal character token
//
// This method assumes currentChar contains a valid, non-whitespace character
// as established by the skipWhitespace() preprocessing step.
func (l *Lexer) dispatchToken() token.Token {
	// Dispatch based on current character to appropriate scanning method
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

	// Handle character-class-based token types
	if unicode.IsDigit(l.currentChar) {
		return l.scanNumericLiteral()
	}

	if unicode.IsLetter(l.currentChar) {
		return l.scanIdentifier()
	}

	// Character doesn't match any recognized token pattern
	return l.createIllegalToken()
}

// Scan performs lexical analysis and returns the next token from the input stream.
// This is the primary method for single-token extraction and forms the foundation
// for all tokenization operations. It handles whitespace skipping, end-of-input
// detection, token position marking, and delegates actual token recognition to dispatchToken().
//
// Returns:
//   - token.Token: The next complete token, which may be:
//   - A valid token of any supported type (IDENT, NUMBER, STRING, operators, etc.)
//   - An ERROR token if lexical errors occur
//   - An EOF token when end of input is reached
//
// Processing sequence:
//  1. Skip any leading whitespace characters
//  2. Handle end-of-input condition (return EOF token)
//  3. Mark the start position of the new token
//  4. Delegate to dispatchToken() for actual token recognition
//  5. Return the resulting token with proper position information
//
// Position tracking:
//   - Token start position is marked after whitespace skipping
//   - Token end position is captured after token scanning completes
//   - Position ranges provide accurate source location for each token
//
// Error handling:
//   - I/O errors during whitespace skipping become ERROR tokens
//   - EOF during whitespace skipping becomes EOF token
//   - Invalid characters become ILLEGAL tokens (via dispatchToken)
//   - Malformed tokens become ERROR tokens (via specialized scanners)
//
// This method advances the lexer state and should be called repeatedly
// to extract all tokens from the input until EOF is encountered.
func (l *Lexer) Scan() token.Token {
	// Skip any leading whitespace before token recognition
	if err := l.skipWhitespace(); err != nil {
		if errors.Is(err, io.EOF) {
			return token.OfEOF(l.position)
		}
		return l.createErrorToken(err)
	}

	// Mark where the current token starts before dispatching
	l.markTokenStart()

	return l.dispatchToken()
}

// Tokens processes the entire input and returns all tokens as a slice.
// This convenience method performs complete tokenization of the input by repeatedly
// calling Scan() until an EOF token is encountered. It's useful for batch processing
// scenarios where all tokens are needed at once rather than streaming tokenization.
//
// Returns:
//   - []token.Token: A slice containing all tokens from the input including the final EOF token
//
// Processing behavior:
//   - Continues tokenization until EOF token is encountered
//   - Includes ERROR and ILLEGAL tokens in the result (doesn't stop on errors)
//   - Always includes EOF token as the final element
//   - Preserves token order exactly as they appear in input
//
// Memory considerations:
//   - Allocates slice to hold all tokens simultaneously
//   - May consume significant memory for large inputs
//   - Consider using Scan() directly for streaming applications
//
// Error handling:
//   - Individual token errors are included in the result slice
//   - Method doesn't fail on lexical errors, allowing error recovery
//   - Caller can examine returned tokens to identify and handle errors
func (l *Lexer) Tokens() []token.Token {
	var allTokens []token.Token

	for {
		currentToken := l.Scan()
		allTokens = append(allTokens, currentToken)

		if currentToken.Kind.Is(token.EOF) {
			break
		}
	}

	return allTokens
}

// Reset reinitializes the lexer to begin tokenizing from the start of the input.
// This method clears all internal state including position counters, character buffers,
// and reader position, effectively returning the lexer to its initial state as if
// it were freshly created. This is useful for re-tokenizing the same input or
// recovering from error states during parsing.
//
// State reset operations:
//   - Position counters reset to (1,1)
//   - Start position reset to initial state
//   - Current character cleared
//   - Reader repositioned to beginning of input
//   - Value buffer cleared and ready for reuse
//
// After calling Reset(), the lexer behaves identically to a newly created lexer
// with the same input string. This enables efficient reuse of lexer instances
// for multiple parsing operations on the same input.
func (l *Lexer) Reset() {
	l.startPosition.Reset()
	l.position.Reset()
	l.currentChar = 0
	l.reader.Reset(l.input)
	l.valueBuffer.Reset()
}
