package image

import "github.com/Tangerg/lynx/ai/core/model"

type Model[O Options, RM ResponseMetadata] interface {
	model.Model[*Prompt[O], *Response[RM]]
}
