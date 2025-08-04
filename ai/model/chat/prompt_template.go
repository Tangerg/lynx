package chat

import (
	"slices"

	"github.com/Tangerg/lynx/ai/commons/content"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/pkg/text"
)

type PromptTemplate struct {
	renderer *text.Renderer
	media    []*content.Media
}

func NewPromptTemplate() *PromptTemplate {
	return &PromptTemplate{
		renderer: text.NewRenderer(),
		media:    make([]*content.Media, 0),
	}
}

func (p *PromptTemplate) WithTemplate(template string) *PromptTemplate {
	p.renderer.WithTemplate(template)
	return p
}

func (p *PromptTemplate) WithVariable(name string, value any) *PromptTemplate {
	p.renderer.WithVariable(name, value)
	return p
}

func (p *PromptTemplate) WithVariables(variables map[string]any) *PromptTemplate {
	p.renderer.WithVariables(variables)
	return p
}

func (p *PromptTemplate) WithMedia(media ...*content.Media) *PromptTemplate {
	p.media = append(p.media, media...)
	return p
}

func (p *PromptTemplate) Clone() *PromptTemplate {
	return &PromptTemplate{
		renderer: p.renderer.Clone(),
		media:    slices.Clone(p.media),
	}
}

func (p *PromptTemplate) RenderSystemMessage() (*messages.SystemMessage, error) {
	contentText, err := p.renderer.Render()
	if err != nil {
		return nil, err
	}

	return messages.NewSystemMessage(contentText), nil
}

func (p *PromptTemplate) RenderUserMessage() (*messages.UserMessage, error) {
	contentText, err := p.renderer.Render()
	if err != nil {
		return nil, err
	}

	return messages.NewUserMessage(messages.UserMessageParam{
		Text:  contentText,
		Media: p.media,
	}), nil
}
