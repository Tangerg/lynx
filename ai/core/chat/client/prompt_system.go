package client

type PromptSystemSpec interface {
	Text(text string) PromptSystemSpec
	Param(k string, v any) PromptSystemSpec
	Params(m map[string]any) PromptSystemSpec
}

var _ PromptSystemSpec = (*DefaultPromptSystemSpec)(nil)

type DefaultPromptSystemSpec struct {
	text   string
	params map[string]any
}

func NewDefaultPromptSystemSpec() *DefaultPromptSystemSpec {
	return &DefaultPromptSystemSpec{
		params: make(map[string]any),
	}
}

func (d *DefaultPromptSystemSpec) Text(text string) PromptSystemSpec {
	d.text = text
	return d
}

func (d *DefaultPromptSystemSpec) Param(k string, v any) PromptSystemSpec {
	d.params[k] = v
	return d
}

func (d *DefaultPromptSystemSpec) Params(m map[string]any) PromptSystemSpec {
	for k, v := range m {
		d.params[k] = v
	}
	return d
}
