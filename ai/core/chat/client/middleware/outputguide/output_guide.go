package outputguide

import (
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	pkgSystem "github.com/Tangerg/lynx/pkg/system"
)

const (
	FormatKey         = "response_format"
	formatContentKey  = "lynx_ai_soc_response_format"
	formatPlaceholder = "{{." + formatContentKey + "}}"
)

func New[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() middleware.Middleware[O, M] {
	return func(ctx *middleware.Context[O, M]) error {
		format, ok := ctx.Get(FormatKey)
		if ok {
			formatStr := cast.ToString(format)
			ctx.Request.SystemText = ctx.Request.SystemText + pkgSystem.LineSeparator() + formatPlaceholder
			ctx.Request.SetSystemParam(formatContentKey, formatStr)
		}
		return ctx.Next()
	}
}
