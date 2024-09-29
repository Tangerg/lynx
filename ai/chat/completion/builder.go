package completion

import (
	"github.com/Tangerg/lynx/ai/chat/message"
	"github.com/Tangerg/lynx/ai/model"
)

type ResultBuilder[RM ResultMetadata] struct {
	result *Result[RM]
}

func (b *ResultBuilder[RM]) WithContent(content string) *ResultBuilder[RM] {
	return b.WithMessage(message.NewAssisantMessage(content))
}

func (b *ResultBuilder[RM]) WithMessage(msg *message.AssisantMessage) *ResultBuilder[RM] {
	b.result.message = msg
	return b
}

func (b *ResultBuilder[RM]) WithMetadata(meta RM) *ResultBuilder[RM] {
	b.result.metadata = meta
	return b
}

func (b *ResultBuilder[RM]) Build() *Result[RM] {
	return b.result
}

type Builder[RM ResultMetadata] struct {
	completion *Completion[RM]
}

func NewBuilder[RM ResultMetadata]() *Builder[RM] {
	return &Builder[RM]{
		completion: &Completion[RM]{
			results: make([]model.Result[*message.AssisantMessage, RM], 0),
		},
	}
}

func (b *Builder[RM]) NewResultBuilder() *ResultBuilder[RM] {
	return &ResultBuilder[RM]{
		result: &Result[RM]{},
	}
}

func (b *Builder[RM]) WithResults(results ...model.Result[*message.AssisantMessage, RM]) *Builder[RM] {
	b.completion.results = append(b.completion.results, results...)
	return b
}

func (b *Builder[RM]) WithMetadata(metadata model.ResponseMetadata) *Builder[RM] {
	b.completion.metadata = metadata
	return b
}

func (b *Builder[RM]) Build() *Completion[RM] {
	return b.completion
}
