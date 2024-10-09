package embedding

import (
	"github.com/Tangerg/lynx/ai/core/model"
)

type Model[O Options] interface {
	model.Model[*Request[O], *Response]
}
