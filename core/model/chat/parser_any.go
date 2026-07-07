package chat

import "errors"

var _ StructuredParser[any] = (*AnyParser)(nil)

// AnyParser is a type-erased [StructuredParser]: it forwards
// Instructions verbatim and runs the supplied parse function. Useful
// when collecting heterogeneous parsers in one slice or storing them on
// an interface field that expects StructuredParser[any].
type AnyParser struct {
	// FormatInstructions is the prompt fragment to inject.
	FormatInstructions string

	// ParseFunction is invoked by [AnyParser.Parse]. Required.
	ParseFunction func(rawLLMOutput string) (any, error)
}

func (a *AnyParser) Instructions() string { return a.FormatInstructions }

func (a *AnyParser) Parse(rawLLMOutput string) (any, error) {
	if a.ParseFunction == nil {
		return nil, errors.New("chat.AnyParser.Parse: ParseFunction is not initialized")
	}
	return a.ParseFunction(rawLLMOutput)
}

// WrapParserAsAny adapts a typed [StructuredParser] into [*AnyParser].
// The wrapped parser's Instructions are captured at wrap time; if the
// wrapped parser regenerates instructions later, the wrapper will not
// observe the change.
func WrapParserAsAny[T any](parser StructuredParser[T]) *AnyParser {
	return &AnyParser{
		FormatInstructions: parser.Instructions(),
		ParseFunction: func(rawLLMOutput string) (any, error) {
			return parser.Parse(rawLLMOutput)
		},
	}
}
