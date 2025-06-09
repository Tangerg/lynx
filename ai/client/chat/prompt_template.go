package chat

import (
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/pkg/text"
	"slices"

	"github.com/Tangerg/lynx/ai/commons/content"
)

type SystemPromptTemplate struct {
	renderer *text.Renderer
}

func NewSystemPromptTemplate() *SystemPromptTemplate {
	return &SystemPromptTemplate{
		renderer: text.NewRenderer(),
	}
}

func (s *SystemPromptTemplate) WithTemplate(template string) *SystemPromptTemplate {
	s.renderer.WithTemplate(template)
	return s
}

func (s *SystemPromptTemplate) WithVariable(key string, value any) *SystemPromptTemplate {
	s.renderer.WithVariable(key, value)
	return s
}

func (s *SystemPromptTemplate) WithVariables(variables map[string]any) *SystemPromptTemplate {
	s.renderer.WithVariables(variables)
	return s
}

func (s *SystemPromptTemplate) Clone() *SystemPromptTemplate {
	return &SystemPromptTemplate{
		renderer: s.renderer.Clone(),
	}
}

func (s *SystemPromptTemplate) RenderMessage() (*messages.SystemMessage, error) {
	contentText, err := s.renderer.Render()
	if err != nil {
		return nil, err
	}
	return messages.NewSystemMessage(contentText), nil
}

type UserPromptTemplate struct {
	renderer *text.Renderer
	media    []*content.Media
}

func NewUserPromptTemplate() *UserPromptTemplate {
	return &UserPromptTemplate{
		renderer: text.NewRenderer(),
		media:    make([]*content.Media, 0),
	}
}

func (u *UserPromptTemplate) Media() []*content.Media {
	return u.media
}

func (u *UserPromptTemplate) WithTemplate(template string) *UserPromptTemplate {
	u.renderer.WithTemplate(template)
	return u
}

func (u *UserPromptTemplate) WithVariable(key string, value any) *UserPromptTemplate {
	u.renderer.WithVariable(key, value)
	return u
}

func (u *UserPromptTemplate) WithVariables(variables map[string]any) *UserPromptTemplate {
	u.renderer.WithVariables(variables)
	return u
}

func (u *UserPromptTemplate) WithMedia(m ...*content.Media) *UserPromptTemplate {
	u.media = append(u.media, m...)
	return u
}

func (u *UserPromptTemplate) Clone() *UserPromptTemplate {
	return &UserPromptTemplate{
		renderer: u.renderer.Clone(),
		media:    slices.Clone(u.media),
	}
}

func (u *UserPromptTemplate) RenderMessage() (*messages.UserMessage, error) {
	contentText, err := u.renderer.Render()
	if err != nil {
		return nil, err
	}
	return messages.NewUserMessage(contentText, u.media), nil
}
