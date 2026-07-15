package filter

import "fmt"

// SyntaxError describes invalid filter text and its source position.
type SyntaxError struct {
	Position Position
	Token    string
	Message  string
}

func newSyntaxError(position Position, token, message string) *SyntaxError {
	return &SyntaxError{Position: position, Token: token, Message: message}
}

func (e *SyntaxError) Error() string {
	if e == nil {
		return "filter: syntax error"
	}
	if e.Token == "" {
		return fmt.Sprintf("filter: %s at %s", e.Message, e.Position)
	}
	return fmt.Sprintf("filter: %s at %s near %q", e.Message, e.Position, e.Token)
}
