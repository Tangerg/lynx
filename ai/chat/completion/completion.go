package completion

import (
	"github.com/Tangerg/lynx/ai/chat/message"
	"github.com/Tangerg/lynx/ai/model"
)

type Completion[Msg *message.AssisantMessage, RM ResultMetadata] struct {
	metadata model.ResponseMetadata
	results  []model.Result[Msg, RM]
}

func (c *Completion[Msg, RM]) Result() model.Result[Msg, RM] {
	return c.results[0]
}

func (c *Completion[Msg, RM]) Results() []model.Result[Msg, RM] {
	return c.results
}

func (c *Completion[Msg, RM]) Metadata() model.ResponseMetadata {
	return c.metadata
}
