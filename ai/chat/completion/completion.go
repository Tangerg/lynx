package completion

import (
	"github.com/Tangerg/lynx/ai/chat/message"
	"github.com/Tangerg/lynx/ai/model"
)

type Completion[RM ResultMetadata] struct {
	metadata model.ResponseMetadata
	results  []model.Result[*message.AssisantMessage, RM]
}

func (c *Completion[RM]) Result() model.Result[*message.AssisantMessage, RM] {
	if len(c.results) == 0 {
		return nil
	}
	return c.results[0]
}

func (c *Completion[RM]) Results() []model.Result[*message.AssisantMessage, RM] {
	return c.results
}

func (c *Completion[RM]) Metadata() model.ResponseMetadata {
	return c.metadata
}
