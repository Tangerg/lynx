package api

import (
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type CallAroundAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	Advisor
	AroundCall(ctx *Context[O, M], chain AroundAdvisorChain[O, M]) error
}
