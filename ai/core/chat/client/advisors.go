package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
)

type Advisors interface {
	Advisors() []api.Advisor
	Param(key string) (any, bool)
	Params() map[string]any
	SetAdvisors(advisors ...api.Advisor) Advisors
	SetParam(k string, v any) Advisors
	SetParams(m map[string]any) Advisors
}

func NewDefaultAdvisors() *DefaultAdvisors {
	return &DefaultAdvisors{
		params: make(map[string]any),
	}
}

var _ Advisors = (*DefaultAdvisors)(nil)

type DefaultAdvisors struct {
	advisors []api.Advisor
	params   map[string]any
}

func (d *DefaultAdvisors) Advisors() []api.Advisor {
	return d.advisors
}

func (d *DefaultAdvisors) Param(key string) (any, bool) {
	v, ok := d.params[key]
	return v, ok
}

func (d *DefaultAdvisors) Params() map[string]any {
	return d.params
}

func (d *DefaultAdvisors) SetAdvisors(advisors ...api.Advisor) Advisors {
	d.advisors = append(d.advisors, advisors...)
	return d
}

func (d *DefaultAdvisors) SetParam(k string, v any) Advisors {
	d.params[k] = v
	return d
}

func (d *DefaultAdvisors) SetParams(m map[string]any) Advisors {
	for k, v := range m {
		d.params[k] = v
	}
	return d
}
