package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	pkgSystem "github.com/Tangerg/lynx/pkg/system"
)

var _ api.CallAroundAdvisor = (*requestAdvisor)(nil)
var _ api.StreamAroundAdvisor = (*requestAdvisor)(nil)

type requestAdvisor struct {
}

func newRequestAdvisor() *requestAdvisor {
	return &requestAdvisor{}
}

func (r *requestAdvisor) Name() string {
	return "requestAdvisor"
}

func (r *requestAdvisor) doGetPrompt(ctx *api.Context) (*prompt.ChatPrompt[prompt.ChatOptions], error) {
	messages := ctx.Request.Messages()
	processedSystemText := ctx.Request.SystemText()
	if len(ctx.Request.SystemParams()) > 0 {
		// TODO prompt template
		messages = append(messages, message.NewSystemMessage(processedSystemText))
	}
	processedUserText := ctx.Request.UserText()
	_, ok := ctx.Param("formatParam")
	if ok {
		processedUserText = processedUserText + pkgSystem.LineSeparator() + "{lynx_ai_soc_format}"

	}
	if processedUserText != "" {
		messages = append(messages, message.NewUserMessage(processedUserText))
		// TODO prompt template
	}

	return prompt.
		NewChatPromptBuilder[prompt.ChatOptions]().
		WithMessages(messages...).
		WithOptions(ctx.Request.ChatOptions()).
		Build()
}

func (r *requestAdvisor) AroundCall(ctx *api.Context, _ api.AroundAdvisorChain) error {
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

func (r *requestAdvisor) AroundStream(ctx *api.Context, _ api.AroundAdvisorChain) error {
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
