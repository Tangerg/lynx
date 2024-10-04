package client

import (
	"strings"
)

type SystemPrompt interface {
	Text() string
	Param(key string) (any, bool)
	Params() map[string]any
	SetText(text string) SystemPrompt
	SetParam(k string, v any) SystemPrompt
	SetParams(m map[string]any) SystemPrompt
}

var _ SystemPrompt = (*DefaultSystemPrompt)(nil)

type DefaultSystemPrompt struct {
	text   string
	params map[string]any
}

func (d *DefaultSystemPrompt) Text() string {
	return d.text
}

func (d *DefaultSystemPrompt) Param(key string) (any, bool) {
	v, ok := d.params[key]
	return v, ok
}

func (d *DefaultSystemPrompt) Params() map[string]any {
	return d.params
}

func (d *DefaultSystemPrompt) SetText(text string) SystemPrompt {
	d.text = strings.TrimSpace(text)
	return d
}

func (d *DefaultSystemPrompt) SetParam(k string, v any) SystemPrompt {
	d.params[k] = v
	return d
}

func (d *DefaultSystemPrompt) SetParams(m map[string]any) SystemPrompt {
	for k, v := range m {
		d.params[k] = v
	}
	return d
}

func NewDefaultSystemPrompt() *DefaultSystemPrompt {
	return &DefaultSystemPrompt{
		params: make(map[string]any),
	}
}
