package templaterender

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	xstrings "github.com/Tangerg/lynx/pkg/strings"
)

func renderText(text string, params map[string]any) string {
	if text == "" || len(params) == 0 {
		return text
	}

	tpl := xstrings.NewTextTemplate()
	err := tpl.ExecuteMap(text, params)
	if err == nil {
		text = tpl.Render()
	}
	return text
}

func New[O request.ChatRequestOptions, M result.ChatResultMetadata]() middleware.Middleware[O, M] {
	return func(ctx *middleware.Context[O, M]) error {
		ctx.Request.UserText = renderText(ctx.Request.UserText, ctx.Request.UserParams)
		ctx.Request.SystemText = renderText(ctx.Request.SystemText, ctx.Request.SystemParams)
		return ctx.Next()
	}
}
