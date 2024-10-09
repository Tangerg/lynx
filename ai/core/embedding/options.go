package embedding

import "github.com/Tangerg/lynx/ai/core/model"

type Options interface {
	model.Options
	Model() string
	Dimensions() int
}
