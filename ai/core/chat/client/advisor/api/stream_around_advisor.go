package api

import (
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type StreamAroundAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	Advisor
	AroundStream(ctx *Context[O, M], chain AroundAdvisorChain[O, M]) error
}
