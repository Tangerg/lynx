package completion

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/model"
)

type Completion[RM metadata.GenerationMetadata] struct {
	metadata model.ResponseMetadata
	results  []model.Result[*message.AssistantMessage, RM]
}

func (c *Completion[RM]) Result() model.Result[*message.AssistantMessage, RM] {
	if len(c.results) == 0 {
		return nil
	}
	return c.results[0]
}

func (c *Completion[RM]) Results() []model.Result[*message.AssistantMessage, RM] {
	return c.results
}

func (c *Completion[RM]) Metadata() model.ResponseMetadata {
	return c.metadata
}

type Builder[RM metadata.GenerationMetadata] struct {
	completion *Completion[RM]
}

func NewBuilder[RM metadata.GenerationMetadata]() *Builder[RM] {
	return &Builder[RM]{
		completion: &Completion[RM]{
			results: make([]model.Result[*message.AssistantMessage, RM], 0),
		},
	}
}

func (b *Builder[RM]) NewGenerationBuilder() *GenerationBuilder[RM] {
	return &GenerationBuilder[RM]{
		result: &Generation[RM]{},
	}
}

func (b *Builder[RM]) WithGenerations(gens ...*Generation[RM]) *Builder[RM] {
	for _, gen := range gens {
		b.completion.results = append(b.completion.results, gen)
	}
	return b
}

func (b *Builder[RM]) WithMetadata(metadata model.ResponseMetadata) *Builder[RM] {
	b.completion.metadata = metadata
	return b
}

func (b *Builder[RM]) Build() *Completion[RM] {
	return b.completion
}
