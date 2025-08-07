package embedding

import (
	"github.com/Tangerg/lynx/ai/model"
)

type Options interface {
	model.Options
	Dimensions() *int64
}
