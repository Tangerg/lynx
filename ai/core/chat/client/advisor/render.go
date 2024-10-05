package advisor

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

var _ api.RequestAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*RenderAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type RenderAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
}

func NewRenderAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() *RenderAdvisor[O, M] {
	return &RenderAdvisor[O, M]{}
}

func (r *RenderAdvisor[O, M]) Name() string {
	return "RenderAdvisor"
}

func (r *RenderAdvisor[O, M]) AdviseRequest(ctx *api.Context[O, M]) error {
	systemText := r.renderSystemText(ctx)
	userText := r.renderUserText(ctx)

	ctx.Request = api.
		NewAdvisedRequestBuilder[O, M]().
		FromAdvisedRequest(ctx.Request).
		WithSystemText(systemText).
		WithUserText(userText).
		Build()

	return nil
}

func (r *RenderAdvisor[O, M]) renderSystemText(ctx *api.Context[O, M]) string {
	systemText := ctx.Request.SystemText()
	systemParams := ctx.Request.SystemParams()
	return r.renderText(systemText, systemParams)
}

func (r *RenderAdvisor[O, M]) renderUserText(ctx *api.Context[O, M]) string {
	userText := ctx.Request.UserText()
	userParams := ctx.Request.UserParams()
	return r.renderText(userText, userParams)
}

func (r *RenderAdvisor[O, M]) renderText(text string, params map[string]any) string {
	if text == "" {
		return text
	}

	if len(params) > 0 {
		tpl := prompt.NewTemplate()
		err := tpl.Execute(text, params)
		if err == nil {
			text = tpl.Render()
		}
	}

	return text
}
