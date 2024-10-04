package advisor

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	pkgSystem "github.com/Tangerg/lynx/pkg/system"
)

var _ api.CallAroundAdvisor = (*DialAdvisor)(nil)
var _ api.StreamAroundAdvisor = (*DialAdvisor)(nil)

type DialAdvisor struct {
}

func NewDialAdvisor() *DialAdvisor {
	return &DialAdvisor{}
}

func (r *DialAdvisor) Name() string {
	return "DialAdvisor"
}

func (r *DialAdvisor) doGetPrompt(ctx *api.Context) (*prompt.ChatPrompt[prompt.ChatOptions], error) {
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
		userParams[".lynx_ai_soc_format"] = formatParam
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
		NewChatPromptBuilder[prompt.ChatOptions]().
		WithMessages(messages...).
		WithOptions(ctx.Request.ChatOptions()).
		Build()
}

func (r *DialAdvisor) AroundCall(ctx *api.Context, _ api.AroundAdvisorChain) error {
	p, err := r.doGetPrompt(ctx)
	if err != nil {
		return err
	}
	resp, err := ctx.Request.ChatModel().Call(ctx.Context(), p)
	if err != nil {
		return err
	}
	ctx.Response = resp
	return nil
}

func (r *DialAdvisor) AroundStream(ctx *api.Context, _ api.AroundAdvisorChain) error {
	p, err := r.doGetPrompt(ctx)
	if err != nil {
		return err
	}
	resp, err := ctx.Request.ChatModel().Stream(ctx.Context(), p)
	if err != nil {
		return err
	}
	ctx.Response = resp
	return nil
}
