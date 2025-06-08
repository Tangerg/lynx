package chat

import (
	"maps"
	"slices"

	"github.com/Tangerg/lynx/ai/commons/content"
)

type SystemPromptTemplate struct {
	template  string
	variables map[string]any
}

func NewSystemPromptTemplate() *SystemPromptTemplate {
	return &SystemPromptTemplate{
		variables: make(map[string]any),
	}
}

func (s *SystemPromptTemplate) Template() string {
	return s.template
}

func (s *SystemPromptTemplate) Variable(key string) (any, bool) {
	val, ok := s.variables[key]
	return val, ok
}

func (s *SystemPromptTemplate) Variables() map[string]any {
	return maps.Clone(s.variables)
}

func (s *SystemPromptTemplate) SetTemplate(template string) *SystemPromptTemplate {
	if template != "" {
		s.template = template
	}
	return s
}

func (s *SystemPromptTemplate) SetVariable(key string, value any) *SystemPromptTemplate {
	s.variables[key] = value
	return s
}

func (s *SystemPromptTemplate) SetVariables(variables map[string]any) *SystemPromptTemplate {
	if len(variables) > 0 {
		maps.Copy(s.variables, variables)
	}
	return s
}

func (s *SystemPromptTemplate) Clone() *SystemPromptTemplate {
	return &SystemPromptTemplate{
		template:  s.template,
		variables: maps.Clone(s.variables),
	}
}

type UserPromptTemplate struct {
	template  string
	variables map[string]any
	media     []*content.Media
}

func NewUserPromptTemplate() *UserPromptTemplate {
	return &UserPromptTemplate{
		variables: make(map[string]any),
		media:     make([]*content.Media, 0),
	}
}

func (u *UserPromptTemplate) Template() string {
	return u.template
}

func (u *UserPromptTemplate) Variable(key string) (any, bool) {
	val, ok := u.variables[key]
	return val, ok
}

func (u *UserPromptTemplate) Variables() map[string]any {
	return maps.Clone(u.variables)
}

func (u *UserPromptTemplate) Media() []*content.Media {
	return slices.Clone(u.media)
}

func (u *UserPromptTemplate) SetTemplate(template string) *UserPromptTemplate {
	u.template = template
	return u
}

func (u *UserPromptTemplate) SetVariable(key string, value any) *UserPromptTemplate {
	u.variables[key] = value
	return u
}

func (u *UserPromptTemplate) SetVariables(variables map[string]any) *UserPromptTemplate {
	if len(variables) > 0 {
		maps.Copy(u.variables, variables)
	}
	return u
}

func (u *UserPromptTemplate) SetMedia(m ...*content.Media) *UserPromptTemplate {
	if len(m) > 0 {
		u.media = append(u.media, m...)
	}
	return u
}

func (u *UserPromptTemplate) Clone() *UserPromptTemplate {
	return &UserPromptTemplate{
		template:  u.template,
		variables: maps.Clone(u.variables),
		media:     slices.Clone(u.media),
	}
}
