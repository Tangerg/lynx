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

var _ Advisors = (*DefaultAdvisorSpec)(nil)

type DefaultAdvisorSpec struct {
	advisors []api.Advisor
	params   map[string]any
}

func (d *DefaultAdvisorSpec) Advisors() []api.Advisor {
	return d.advisors
}

func (d *DefaultAdvisorSpec) Param(key string) (any, bool) {
	v, ok := d.params[key]
	return v, ok
}

func (d *DefaultAdvisorSpec) Params() map[string]any {
	return d.params
}

func (d *DefaultAdvisorSpec) SetAdvisors(advisors ...api.Advisor) Advisors {
	d.advisors = append(d.advisors, advisors...)
	return d
}

func (d *DefaultAdvisorSpec) SetParam(k string, v any) Advisors {
	d.params[k] = v
	return d
}

func (d *DefaultAdvisorSpec) SetParams(m map[string]any) Advisors {
	for k, v := range m {
		d.params[k] = v
	}
	return d
}

func NewDefaultAdvisors() *DefaultAdvisorSpec {
	return &DefaultAdvisorSpec{
		params: make(map[string]any),
	}
}
