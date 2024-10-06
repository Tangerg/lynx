package middleware

import (
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type Middleware[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] func(ctx *Context[O, M]) error
