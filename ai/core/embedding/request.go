package embedding

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.Request[[]string, Options] = (*Request[Options])(nil)

type Request[O Options] struct {
	inputs  []string
	options O
}

func (r *Request[O]) Instructions() []string {
	return r.inputs
}

func (r *Request[O]) Options() O {
	return r.options
}

func NewRequest[O Options](inputs []string, o O) *Request[O] {
	return &Request[O]{
		inputs:  inputs,
		options: o,
	}
}
