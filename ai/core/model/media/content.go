package media

import "github.com/Tangerg/lynx/ai/core/model"

type Content interface {
	model.Content
	Media() []*Media
}
