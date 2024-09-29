package completion

import (
	"github.com/Tangerg/lynx/ai/chat/message"
)

type Result[M ResultMetadata] struct {
	message  *message.AssisantMessage
	metadata M
}

func (r *Result[M]) Output() *message.AssisantMessage {
	return r.message
}

func (r *Result[M]) Metadata() M {
	return r.metadata
}
