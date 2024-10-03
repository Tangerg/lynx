package client

type PromptUserSpec interface {
	Text(text string) PromptUserSpec
	Param(k string, v any) PromptUserSpec
	Params(m map[string]any) PromptUserSpec
}

var _ PromptUserSpec = (*DefaultPromptUserSpec)(nil)

type DefaultPromptUserSpec struct {
	text   string
	params map[string]any
}

func NewDefaultPromptUserSpec() *DefaultPromptUserSpec {
	return &DefaultPromptUserSpec{
		params: make(map[string]any),
	}
}

func (d *DefaultPromptUserSpec) Text(text string) PromptUserSpec {
	d.text = text
	return d
}

func (d *DefaultPromptUserSpec) Param(k string, v any) PromptUserSpec {
	d.params[k] = v
	return d
}

func (d *DefaultPromptUserSpec) Params(m map[string]any) PromptUserSpec {
	for k, v := range m {
		d.params[k] = v
	}
	return d
}
