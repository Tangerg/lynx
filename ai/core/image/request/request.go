package request

import (
	"github.com/Tangerg/lynx/ai/core/image/message"
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Request[[]*message.ImageMessage, ImageRequestOptions] = (*ImageRequest[ImageRequestOptions])(nil)

type ImageRequest[O ImageRequestOptions] struct {
	options  O
	messages []*message.ImageMessage
}

func (r ImageRequest[O]) Instructions() []*message.ImageMessage {
	return r.messages
}

func (r ImageRequest[O]) Options() O {
	return r.options
}
