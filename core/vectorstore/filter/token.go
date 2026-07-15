package filter

type tokenKind uint8

const (
	tokenInvalid tokenKind = iota
	tokenEOF
	tokenIdent
	tokenNumber
	tokenString
	tokenTrue
	tokenFalse
	tokenEqual
	tokenNotEqual
	tokenLess
	tokenLessEqual
	tokenGreater
	tokenGreaterEqual
	tokenAnd
	tokenOr
	tokenNot
	tokenIn
	tokenLike
	tokenIs
	tokenNull
	tokenLeftParen
	tokenRightParen
	tokenLeftBracket
	tokenRightBracket
	tokenComma
)

type lexeme struct {
	kind       tokenKind
	literal    string
	start, end Position
}

func (k tokenKind) string() string {
	switch k {
	case tokenEOF:
		return "end of input"
	case tokenIdent:
		return "identifier"
	case tokenNumber:
		return "number"
	case tokenString:
		return "string"
	case tokenTrue, tokenFalse:
		return "boolean"
	case tokenEqual:
		return "=="
	case tokenNotEqual:
		return "!="
	case tokenLess:
		return "<"
	case tokenLessEqual:
		return "<="
	case tokenGreater:
		return ">"
	case tokenGreaterEqual:
		return ">="
	case tokenAnd:
		return "AND"
	case tokenOr:
		return "OR"
	case tokenNot:
		return "NOT"
	case tokenIn:
		return "IN"
	case tokenLike:
		return "LIKE"
	case tokenIs:
		return "IS"
	case tokenNull:
		return "NULL"
	case tokenLeftParen:
		return "("
	case tokenRightParen:
		return ")"
	case tokenLeftBracket:
		return "["
	case tokenRightBracket:
		return "]"
	case tokenComma:
		return ","
	default:
		return "invalid token"
	}
}
