package embedding

import (
	"github.com/Tangerg/lynx/ai/model"
)

type Model interface {
	model.Model[*Request, *Response]
	Dimensions() int64
	DefaultOptions() Options
}
