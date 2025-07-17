package parser

import (
	"errors"
	"io"
	"strings"
	"unicode"
)

// Lexer performs lexical analysis on input text, breaking it into tokens.
// It maintains the currentToken parsing state including position tracking,
// character reading, and token buffer management.
type Lexer struct {
	input       string          // The complete input string being tokenized
	currentPos  Position        // Current position in the input (line and column)
	currentChar rune            // The currentToken character being processed
	reader      *strings.Reader // Reader for efficient character-by-character processing
	buffer      strings.Builder // Reusable buffer for building token values
}

// NewLexer creates a new lexer instance for the given input string.
// It initializes the lexer state and positions it at the beginning of the input.
// Returns an error if the input is empty, as there's nothing to tokenize.
func NewLexer(input string) (*Lexer, error) {
	if len(input) == 0 {
		return nil, errors.New("input is empty")
	}

	return &Lexer{
		input: input,
		currentPos: Position{
			lineNum:   1,
			columnNum: 1,
		},
		currentChar: 0,
		reader:      strings.NewReader(input),
		buffer:      strings.Builder{},
	}, nil
}

func (l *Lexer) Reset() {
	l.currentPos = Position{
		lineNum:   1,
		columnNum: 1,
	}
	l.currentChar = 0
	l.reader.Reset(l.input)
	l.buffer.Reset()
}

// readRune reads the nextToken rune from the input and updates the currentToken position.
// It properly handles newline characters by incrementing the line number
// and resetting the column number to 1. For other characters, it simply
// increments the column number.
func (l *Lexer) readRune() error {
	char, _, err := l.reader.ReadRune()
	if err != nil {
		return err
	}

	l.currentChar = char

	// Handle position tracking for newlines and regular characters
	if char == '\n' {
		l.currentPos.lineNum++
		l.currentPos.columnNum = 1
	} else {
		l.currentPos.columnNum++
	}

	return nil
}

// peekRune returns the nextToken rune without consuming it from the input.
// This is useful for lookahead operations where we need to check what
// comes nextToken without advancing the lexer position. The rune is immediately
// put back using UnreadRune() to maintain the currentToken position.
func (l *Lexer) peekRune() (rune, error) {
	nextChar, _, err := l.reader.ReadRune()
	if err != nil {
		return 0, err
	}

	// Put the character back so it can be read again later
	if err = l.reader.UnreadRune(); err != nil {
		return 0, err
	}

	return nextChar, nil
}

// skipSpace skips whitespace characters and positions the lexer at the nextToken non-space character.
// It continues reading characters until it finds a non-whitespace character or reaches EOF.
// This is essential for ignoring insignificant whitespace between tokens.
func (l *Lexer) skipSpace() error {
	for {
		if err := l.readRune(); err != nil {
			return err
		}

		// Stop when we find a non-whitespace character
		if !unicode.IsSpace(l.currentChar) {
			return nil
		}
	}
}

// escapeSequences maps escape sequences to their corresponding characters.
// This lookup table is used when parsing string literals to convert
// escape sequences like '\n' into their actual character representations.
var escapeSequences = map[rune]rune{
	'n':  '\n', // newline
	't':  '\t', // tab
	'r':  '\r', // carriage return
	'\\': '\\', // backslash
	'\'': '\'', // single quote
}

// readString reads a string literal enclosed in single quotes, handling escape sequences.
// It processes characters until it finds the closing quote, properly handling
// escape sequences using the escapeSequences map. The buffer is reset after use
// to ensure clean state for subsequent string parsing.
func (l *Lexer) readString() Token {
	defer l.buffer.Reset()

	for {
		if err := l.readRune(); err != nil {
			return NewErrorToken(err, l.currentPos)
		}

		// End of string literal
		if l.currentChar == '\'' {
			break
		}

		// Handle regular characters (not escape sequences)
		if l.currentChar != '\\' {
			l.buffer.WriteRune(l.currentChar)
			continue
		}

		if err := l.readRune(); err != nil {
			return NewErrorToken(err, l.currentPos)
		}

		// Look up the escape sequence in our map
		if escapedChar, exists := escapeSequences[l.currentChar]; exists {
			l.buffer.WriteRune(escapedChar)
		} else {
			// If not a recognized escape sequence, use the character as-is
			l.buffer.WriteRune(l.currentChar)
		}
	}

	return NewToken(STRING, l.buffer.String(), l.currentPos)
}

// readNegativeNumber reads a negative number token starting with a minus sign.
// It performs lookahead to ensure the minus sign is followed by a digit,
// otherwise it treats the minus as an illegal token. This prevents confusion
// between negative numbers and minus operators.
func (l *Lexer) readNegativeNumber() Token {
	nextChar, err := l.peekRune()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return NewIllegalToken(l.currentChar, l.currentPos)
		}
		return NewErrorToken(err, l.currentPos)
	}

	// A minus sign must be followed by a digit to be a negative number
	if !unicode.IsDigit(nextChar) {
		return NewIllegalToken(l.currentChar, l.currentPos)
	}

	if err = l.readRune(); err != nil {
		return NewErrorToken(err, l.currentPos)
	}

	// Parse the number portion
	numberToken := l.readNumber()
	if numberToken.Kind() == ERROR {
		return numberToken
	}

	// Prepend the minus sign to create the negative value
	negativeValue := "-" + numberToken.Value()
	return NewToken(NUMBER, negativeValue, l.currentPos)
}

// readNumber reads a numeric literal, supporting both integers and decimal numbers.
// It first reads the integer part, then checks for a decimal point to determine
// if it should continue reading the fractional part. The buffer is reset after
// use to maintain clean state.
func (l *Lexer) readNumber() Token {
	defer l.buffer.Reset()

	// Read the integer part digit by digit
	for {
		l.buffer.WriteRune(l.currentChar)

		nextChar, err := l.peekRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return NewErrorToken(err, l.currentPos)
		}

		// Stop if nextToken character is not a digit
		if !unicode.IsDigit(nextChar) {
			break
		}

		if err = l.readRune(); err != nil {
			return NewErrorToken(err, l.currentPos)
		}
	}

	// Check for decimal point to handle floating-point numbers
	nextChar, err := l.peekRune()
	if err != nil && !errors.Is(err, io.EOF) {
		return NewErrorToken(err, l.currentPos)
	}

	// If we found a decimal point, read the fractional part
	if err == nil && nextChar == '.' {
		if err = l.readRune(); err != nil {
			return NewErrorToken(err, l.currentPos)
		}

		// Read the fractional part digit by digit
		for {
			l.buffer.WriteRune(l.currentChar)

			nextChar, err = l.peekRune()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return NewErrorToken(err, l.currentPos)
			}

			// Stop if nextToken character is not a digit
			if !unicode.IsDigit(nextChar) {
				break
			}

			if err = l.readRune(); err != nil {
				return NewErrorToken(err, l.currentPos)
			}
		}
	}

	return NewToken(NUMBER, l.buffer.String(), l.currentPos)
}

// readKeywordsOrIdentifier reads keywords or identifiers, checking against reserved words.
// It reads a sequence of letters, digits, and underscores, then uses LookupTokenKind
// to determine if it's a keyword or identifier. The buffer is reset after use to
// maintain clean state.
func (l *Lexer) readKeywordsOrIdentifier() Token {
	defer l.buffer.Reset()

	// Read identifier characters (letters, digits, underscores)
	for {
		l.buffer.WriteRune(l.currentChar)

		nextChar, err := l.peekRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return NewErrorToken(err, l.currentPos)
		}

		// Stop if nextToken character is not part of an identifier
		if !unicode.IsLetter(nextChar) &&
			!unicode.IsDigit(nextChar) &&
			nextChar != '_' {
			break
		}

		if err = l.readRune(); err != nil {
			return NewErrorToken(err, l.currentPos)
		}
	}

	tokenValue := l.buffer.String()
	// Check if this is a keyword or just an identifier
	tokenKind := LookupTokenKind(tokenValue)

	return NewToken(tokenKind, tokenValue, l.currentPos)
}

// readTwoCharOperator reads operators that can be either single or double character.
// This is a generic function for handling operators like '>', '>=', '<', '<=', etc.
// It performs lookahead to check if the operator should be treated as a single
// or double character operator based on the expected second character.
func (l *Lexer) readTwoCharOperator(firstChar, expectedSecondChar rune,
	singleCharKind, doubleCharKind TokenKind) Token {

	nextChar, err := l.peekRune()
	if err != nil {
		if errors.Is(err, io.EOF) {
			// End of input, return single character operator
			return NewToken(singleCharKind, string(firstChar), l.currentPos)
		}
		return NewErrorToken(err, l.currentPos)
	}

	// If nextToken character doesn't match expected, return single character operator
	if nextChar != expectedSecondChar {
		return NewToken(singleCharKind, string(firstChar), l.currentPos)
	}

	if err = l.readRune(); err != nil {
		return NewErrorToken(err, l.currentPos)
	}

	operatorValue := string(firstChar) + string(expectedSecondChar)
	return NewToken(doubleCharKind, operatorValue, l.currentPos)
}

// readExclamationOperator reads the '!=' operator, returning an error if not followed by '='.
// Unlike other operators, '!' by itself is not a valid token in this language,
// so it must be followed by '=' to form the inequality operator '!='.
func (l *Lexer) readExclamationOperator() Token {
	err := l.readRune()
	if err != nil {
		return NewErrorToken(err, l.currentPos)
	}

	// '!' must be followed by '=' to be valid
	if l.currentChar != '=' {
		return NewIllegalToken(l.currentChar, l.currentPos)
	}

	return NewToken(NE, "!=", l.currentPos)
}

// NextToken returns the nextToken token from the input stream.
// This is the main entry point for tokenization. It skips whitespace,
// then uses a switch statement to handle different character types.
// Each case represents a different token type or delegates to specialized
// reading functions for complex tokens.
func (l *Lexer) NextToken() Token {
	// Skip any whitespace before the nextToken token
	err := l.skipSpace()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return NewEOFToken(l.currentPos)
		}
		return NewErrorToken(err, l.currentPos)
	}

	// Handle different token types based on currentToken character
	switch l.currentChar {
	case '=':
		return NewToken(EQ, "=", l.currentPos)
	case '!':
		return l.readExclamationOperator() // Must be followed by '='
	case '>':
		return l.readTwoCharOperator('>', '=', GT, GE) // '>' or '>='
	case '<':
		return l.readTwoCharOperator('<', '=', LT, LE) // '<' or '<='
	case ',':
		return NewToken(COMMA, ",", l.currentPos)
	case '\'':
		return l.readString() // String literal
	case '(':
		return NewToken(LPAREN, "(", l.currentPos)
	case ')':
		return NewToken(RPAREN, ")", l.currentPos)
	//case '[':
	//	return NewToken(LBRACKET, "[", l.currentPos)
	//case ']':
	//	return NewToken(RBRACKET, "]", l.currentPos)
	case '-':
		return l.readNegativeNumber() // Negative number
	}

	// Handle numeric literals
	if unicode.IsDigit(l.currentChar) {
		return l.readNumber()
	}

	// Handle keywords and identifiers
	if unicode.IsLetter(l.currentChar) {
		return l.readKeywordsOrIdentifier()
	}

	// If we reach here, it's an unrecognized character
	return NewIllegalToken(l.currentChar, l.currentPos)
}

// Tokens returns all tokens from the input as a slice, including the EOF token.
// This is a convenience method that repeatedly calls NextToken() until EOF
// is reached, collecting all tokens in a slice. This is useful for debugging
// or when you need to process all tokens at once rather than streaming them.
func (l *Lexer) Tokens() []Token {
	var tokenList []Token

	for {
		token := l.NextToken()
		tokenList = append(tokenList, token)

		if token.Kind() == EOF {
			break
		}
	}

	return tokenList
}
