package parser

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

type Lexer struct {
	input    string
	position Position
	current  rune
	reader   *strings.Reader
}

func NewLexer(input string) *Lexer {
	return &Lexer{
		input:  input,
		reader: strings.NewReader(input),
		position: Position{
			line:   1,
			column: 1,
		},
	}
}

func (l *Lexer) readRune() error {
	char, _, err := l.reader.ReadRune()
	if err != nil {
		return err
	}

	l.current = char

	if char == '\n' {
		l.position.line++
		l.position.column = 0
	} else {
		l.position.column++
	}

	return nil
}

func (l *Lexer) peekRune() (rune, error) {
	char, _, err := l.reader.ReadRune()
	if err != nil {
		return 0, err
	}

	if err = l.reader.UnreadRune(); err != nil {
		return 0, err
	}

	return char, nil
}

func (l *Lexer) skipSpace() error {
	for {
		if err := l.readRune(); err != nil {
			return err
		}

		if !unicode.IsSpace(l.current) {
			return nil
		}
	}
}

func (l *Lexer) readString() (Token, error) {
	var sb strings.Builder

	for {
		if err := l.readRune(); err != nil {
			if errors.Is(err, io.EOF) {
				return NewToken(ILLEGAL, fmt.Sprintf("unexpected EOF at %s", l.position.String()), l.position),
					errors.Join(io.EOF, io.ErrUnexpectedEOF, errors.New(l.position.String()))
			}
			return NewToken(ERROR, sb.String(), l.position), err
		}

		if l.current == '\'' {
			break
		}

		if l.current == '\\' {
			if err := l.readRune(); err != nil {
				return NewToken(ERROR, sb.String(), l.position), err
			}

			switch l.current {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case 'r':
				sb.WriteRune('\r')
			case '\\':
				sb.WriteRune('\\')
			case '\'':
				sb.WriteRune('\'')
			default:
				sb.WriteRune(l.current)
			}
		} else {
			sb.WriteRune(l.current)
		}
	}

	return NewToken(STRING, sb.String(), l.position), nil
}

func (l *Lexer) readNumber() (Token, error) {
	var (
		sb      strings.Builder
		isFloat bool
	)

	for {
		sb.WriteRune(l.current)

		next, err := l.peekRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return NewToken(ERROR, sb.String(), l.position), err
		}

		if !unicode.IsDigit(next) {
			break
		}

		if err = l.readRune(); err != nil {
			return NewToken(ERROR, sb.String(), l.position), err
		}
	}

	next, err := l.peekRune()
	if err != nil && !errors.Is(err, io.EOF) {
		return NewToken(ERROR, sb.String(), l.position), err
	}

	if err == nil && next == '.' {
		isFloat = true
		if err = l.readRune(); err != nil {
			return NewToken(ERROR, sb.String(), l.position), err
		}
		sb.WriteRune(l.current)

		for {
			next, err = l.peekRune()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return NewToken(ERROR, sb.String(), l.position), err
			}

			if !unicode.IsDigit(next) {
				break
			}

			if err = l.readRune(); err != nil {
				return NewToken(ERROR, sb.String(), l.position), err
			}
			sb.WriteRune(l.current)
		}
	}

	value := sb.String()
	if isFloat {
		return NewToken(FLOAT, value, l.position), nil
	}
	return NewToken(INT, value, l.position), nil
}

func (l *Lexer) readIdentifier() (Token, error) {
	var sb strings.Builder

	for {
		sb.WriteRune(l.current)

		next, err := l.peekRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return NewToken(ERROR, sb.String(), l.position), err
		}

		if !unicode.IsLetter(next) &&
			!unicode.IsDigit(next) &&
			next != '_' {
			break
		}

		if err = l.readRune(); err != nil {
			return NewToken(ERROR, sb.String(), l.position), err
		}
	}

	value := sb.String()
	tokenType := LookupTokenType(value)
	return NewToken(tokenType, value, l.position), nil
}

func (l *Lexer) readTwoCharOperator(first, second rune, singleType, doubleType TokenType) (Token, error) {
	next, err := l.peekRune()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return NewToken(singleType, string(first), l.position), nil
		}
		return NewToken(ERROR, err.Error(), l.position), err
	}

	if next != second {
		return NewToken(singleType, string(first), l.position), nil
	}

	if err = l.readRune(); err != nil {
		return NewToken(ERROR, err.Error(), l.position), err
	}

	return NewToken(doubleType, string([]rune{first, second}), l.position), nil
}

func (l *Lexer) NextToken() (Token, error) {
	err := l.skipSpace()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return NewToken(EOF, "", l.position), nil
		}
		return NewToken(ERROR, err.Error(), l.position), err
	}

	switch l.current {
	case '=':
		return NewToken(EQ, "=", l.position), nil
	case '!':
		return l.readTwoCharOperator('!', '=', ILLEGAL, NEQ)
	case '>':
		return l.readTwoCharOperator('>', '=', GT, GTE)
	case '<':
		return l.readTwoCharOperator('<', '=', LT, LTE)
	case ',':
		return NewToken(COMMA, ",", l.position), nil
	case '\'':
		return l.readString()
	case '(':
		return NewToken(LPAREN, "(", l.position), nil
	case ')':
		return NewToken(RPAREN, ")", l.position), nil
	case '[':
		return NewToken(LBRACKET, "[", l.position), nil
	case ']':
		return NewToken(RBRACKET, "]", l.position), nil
	case ';':
		return NewToken(SEMICOLON, ";", l.position), nil
	}

	if unicode.IsDigit(l.current) {
		return l.readNumber()
	}

	if unicode.IsLetter(l.current) || l.current == '_' {
		return l.readIdentifier()
	}

	return NewToken(ILLEGAL, string(l.current), l.position), nil
}

func (l *Lexer) Tokenize() ([]Token, error) {
	var tokens []Token

	for {
		token, err := l.NextToken()
		if err != nil {
			return nil, errors.Join(err, fmt.Errorf("lexer error at %s: %s", token.position.String(), token.value))
		}

		tokens = append(tokens, token)

		if token._type == EOF {
			break
		}
	}

	return tokens, nil
}
