package client

import (
	"strings"
)

type UserPrompt interface {
	Text() string
	Param(key string) (any, bool)
	Params() map[string]any
	SetText(text string) UserPrompt
	SetParam(k string, v any) UserPrompt
	SetParams(m map[string]any) UserPrompt
}

func NewDefaultUserPrompt() *DefaultUserPrompt {
	return &DefaultUserPrompt{
		params: make(map[string]any),
	}
}

var _ UserPrompt = (*DefaultUserPrompt)(nil)

type DefaultUserPrompt struct {
	text   string
	params map[string]any
}

func (d *DefaultUserPrompt) Text() string {
	return d.text
}

func (d *DefaultUserPrompt) Param(key string) (any, bool) {
	v, ok := d.params[key]
	return v, ok
}

func (d *DefaultUserPrompt) Params() map[string]any {
	return d.params
}

func (d *DefaultUserPrompt) SetText(text string) UserPrompt {
	d.text = strings.TrimSpace(text)
	return d
}

func (d *DefaultUserPrompt) SetParam(k string, v any) UserPrompt {
	d.params[k] = v
	return d
}

func (d *DefaultUserPrompt) SetParams(m map[string]any) UserPrompt {
	for k, v := range m {
		d.params[k] = v
	}
	return d
}
