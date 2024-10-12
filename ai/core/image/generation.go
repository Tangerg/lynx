package image

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.Result[*Image, GenerationMetadata] = (*Generation[GenerationMetadata])(nil)

type Generation[GM GenerationMetadata] struct {
	metadata GM
	image    *Image
}

func (g *Generation[GM]) Output() *Image {
	return g.image
}

func (g *Generation[GM]) Metadata() GM {
	return g.metadata
}
