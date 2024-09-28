package completion

import (
	"github.com/Tangerg/lynx/ai/chat/message"
)

type Result[Msg *message.AssisantMessage, M ResultMetadata] struct {
	message  Msg
	metadata M
}

func (r *Result[Msg, M]) Output() Msg {
	return r.message
}

func (r *Result[Msg, M]) Metadata() M {
	return r.metadata
}
