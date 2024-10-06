package templaterender

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

func renderText(text string, params map[string]any) string {
	if text == "" || len(params) == 0 {
		return text
	}

	tpl := prompt.NewTemplate()
	err := tpl.Execute(text, params)
	if err == nil {
		text = tpl.Render()
	}
	return text
}

func New[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() middleware.Middleware[O, M] {
	return func(ctx *middleware.Context[O, M]) error {
		ctx.Request.UserText = renderText(ctx.Request.UserText, ctx.Request.UserParams)
		ctx.Request.SystemText = renderText(ctx.Request.SystemText, ctx.Request.SystemParams)
		return ctx.Next()
	}
}
