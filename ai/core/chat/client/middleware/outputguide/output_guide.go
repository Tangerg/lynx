package outputguide

import (
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	pkgSystem "github.com/Tangerg/lynx/pkg/system"
)

const (
	formatContentKey  = "lynx_ai_soc_response_format"
	formatPlaceholder = "{{." + formatContentKey + "}}"
)

func New[O request.ChatRequestOptions, M result.ChatResultMetadata]() middleware.Middleware[O, M] {
	return func(ctx *middleware.Context[O, M]) error {
		format, ok := ctx.Get(middleware.ResponseFormatKey)
		if ok {
			formatStr := cast.ToString(format)
			ctx.Request.UserText = ctx.Request.UserText + pkgSystem.LineSeparator() + formatPlaceholder
			ctx.Request.SetUserParam(formatContentKey, formatStr)
		}
		return ctx.Next()
	}
}
