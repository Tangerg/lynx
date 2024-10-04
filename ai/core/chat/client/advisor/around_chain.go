package advisor

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
)

var _ api.AroundAdvisorChain = (*DefaultAroundChain)(nil)

type DefaultAroundChain struct {
	callAroundAdvisors   []api.CallAroundAdvisor
	streamAroundAdvisors []api.StreamAroundAdvisor
}

func (d *DefaultAroundChain) PushAroundAdvisors(advisors ...api.Advisor) *DefaultAroundChain {
	for _, advisor := range advisors {
		d.PushAroundAdvisor(advisor)
	}
	return d
}

func (d *DefaultAroundChain) PushAroundAdvisor(advisor api.Advisor) *DefaultAroundChain {
	callAroundAdvisor, ok := advisor.(api.CallAroundAdvisor)
	if ok {
		d.callAroundAdvisors = append(d.callAroundAdvisors, callAroundAdvisor)
	}
	streamAroundAdvisor, ok := advisor.(api.StreamAroundAdvisor)
	if ok {
		d.streamAroundAdvisors = append(d.streamAroundAdvisors, streamAroundAdvisor)
	}
	return d
}

func (d *DefaultAroundChain) popCallAroundAdvisor() (api.CallAroundAdvisor, error) {
	if len(d.callAroundAdvisors) == 0 {
		return nil, ErrorChainNoAroundAdvisor
	}
	rv := d.callAroundAdvisors[len(d.callAroundAdvisors)-1]
	d.callAroundAdvisors = d.callAroundAdvisors[:len(d.callAroundAdvisors)-1]
	return rv, nil
}

func (d *DefaultAroundChain) popStreamAroundAdvisor() (api.StreamAroundAdvisor, error) {
	if len(d.streamAroundAdvisors) == 0 {
		return nil, ErrorChainNoAroundAdvisor
	}
	rv := d.streamAroundAdvisors[len(d.streamAroundAdvisors)-1]
	d.streamAroundAdvisors = d.streamAroundAdvisors[:len(d.streamAroundAdvisors)-1]
	return rv, nil
}

func (d *DefaultAroundChain) NextAroundCall(ctx *api.Context) error {
	advisor, err := d.popCallAroundAdvisor()
	if err != nil {
		return err
	}
	return advisor.AroundCall(ctx, d)
}

func (d *DefaultAroundChain) NextAroundStream(ctx *api.Context) error {
	advisor, err := d.popStreamAroundAdvisor()
	if err != nil {
		return err
	}
	return advisor.AroundStream(ctx, d)
}

func NewDefaultAroundChain() *DefaultAroundChain {
	return &DefaultAroundChain{}
}
