package api

import (
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type AroundAdvisorChain[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	NextAroundCall(ctx *Context[O, M]) error
	NextAroundStream(ctx *Context[O, M]) error
}
