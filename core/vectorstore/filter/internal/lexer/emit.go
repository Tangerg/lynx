package lexer

import "github.com/Tangerg/lynx/core/vectorstore/filter/internal/token"

func (l *Lexer) markTokenStart() {
	l.startPosition = l.cursor
	l.startPosition.Column = max(l.startPosition.Column-1, 1)
}

func (l *Lexer) emitEOF() token.Token {
	l.markTokenStart()
	return token.OfEOF(l.startPosition)
}

func (l *Lexer) emitError(err error) token.Token {
	l.markTokenStart()
	return token.OfError(err, l.startPosition)
}

func (l *Lexer) emitIllegal() token.Token {
	l.markTokenStart()
	return token.OfIllegal(l.currentChar, l.startPosition)
}

func (l *Lexer) emitKind(kind token.Kind) token.Token {
	return token.OfKind(kind, l.startPosition, l.cursor)
}

func (l *Lexer) emitLiteral(kind token.Kind, literal string) token.Token {
	return token.OfLiteral(kind, literal, l.startPosition, l.cursor)
}

func (l *Lexer) emitIdent(literal string) token.Token {
	return token.OfIdent(literal, l.startPosition, l.cursor)
}
