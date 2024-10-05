package advisor

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	pkgSystem "github.com/Tangerg/lynx/pkg/system"
)

func NewDialAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() *DialAdvisor[O, M] {
	return &DialAdvisor[O, M]{}
}

var _ api.CallAroundAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DialAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)
var _ api.StreamAroundAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DialAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DialAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct{}

func (r *DialAdvisor[O, M]) Name() string {
	return "DialAdvisor"
}

func (r *DialAdvisor[O, M]) doGetPrompt(ctx *api.Context[O, M]) (*prompt.ChatPrompt[O], error) {
	messages := ctx.Request.Messages()

	systemText := ctx.Request.SystemText()
	if systemText != "" {
		systemParams := ctx.Request.SystemParams()
		if len(systemParams) > 0 {
			tpl := prompt.NewTemplate()
			err := tpl.Execute(systemText, systemParams)
			if err == nil {
				systemText = tpl.Render()
			}
		}
		messages = append(messages, message.NewSystemMessage(systemText))
	}

	userText := ctx.Request.UserText()
	userParams := ctx.Request.UserParams()
	formatParam, ok := ctx.Param("formatParam")
	if ok {
		userText = userText + pkgSystem.LineSeparator() + "{{.lynx_ai_soc_format}}"
		userParams["lynx_ai_soc_format"] = formatParam
	}
	if userText != "" {
		if len(userParams) > 0 {
			tpl := prompt.NewTemplate()
			err := tpl.Execute(userText, userParams)
			if err == nil {
				userText = tpl.Render()
			}
		}
		messages = append(messages, message.NewUserMessage(userText))
	}

	return prompt.
		NewChatPromptBuilder[O]().
		WithMessages(messages...).
		WithOptions(ctx.Request.ChatOptions()).
		Build()
}

func (r *DialAdvisor[O, M]) AroundCall(ctx *api.Context[O, M], _ api.AroundAdvisorChain[O, M]) error {
	p, err := r.doGetPrompt(ctx)
	if err != nil {
		return err
	}
	resp, err := ctx.
		Request.
		ChatModel().
		Call(ctx.Context(), p)
	if err != nil {
		return err
	}
	ctx.Response = resp
	return nil
}

func (r *DialAdvisor[O, M]) AroundStream(ctx *api.Context[O, M], _ api.AroundAdvisorChain[O, M]) error {
	p, err := r.doGetPrompt(ctx)
	if err != nil {
		return err
	}
	resp, err := ctx.
		Request.
		ChatModel().
		Stream(ctx.Context(), p)
	if err != nil {
		return err
	}
	ctx.Response = resp
	return nil
}
