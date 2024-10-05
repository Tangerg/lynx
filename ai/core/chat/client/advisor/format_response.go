package advisor

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	pkgSystem "github.com/Tangerg/lynx/pkg/system"
)

func NewFormatResponseAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](format string) *FormatResponseAdvisor[O, M] {
	return &FormatResponseAdvisor[O, M]{format: format}
}

var _ api.RequestAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*FormatResponseAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type FormatResponseAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	format string
}

func (f *FormatResponseAdvisor[O, M]) Name() string {
	return "FormatResponseAdvisor"
}

func (f *FormatResponseAdvisor[O, M]) AdviseRequest(ctx *api.Context[O, M]) error {
	if f.format == "" {
		return nil
	}

	systemText := ctx.Request.SystemText()
	systemParams := ctx.Request.SystemParams()

	systemText = systemText + pkgSystem.LineSeparator() + "{{.lynx_ai_soc_response_format}}"
	systemParams["lynx_ai_soc_response_format"] = f.format

	ctx.Request = api.
		NewAdvisedRequestBuilder[O, M]().
		FromAdvisedRequest(ctx.Request).
		WithSystemText(systemText).
		WithSystemParam(systemParams).
		Build()

	return nil
}
