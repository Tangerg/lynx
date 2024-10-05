package advisor

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

func NewDefaultAroundChain[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() *DefaultAroundChain[O, M] {
	return &DefaultAroundChain[O, M]{}
}

var _ api.AroundAdvisorChain[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DefaultAroundChain[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DefaultAroundChain[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	callAroundAdvisors   []api.CallAroundAdvisor[O, M]
	streamAroundAdvisors []api.StreamAroundAdvisor[O, M]
}

func (d *DefaultAroundChain[O, M]) PushAroundAdvisors(advisors ...api.Advisor) *DefaultAroundChain[O, M] {
	for _, advisor := range advisors {
		d.PushAroundAdvisor(advisor)
	}
	return d
}

func (d *DefaultAroundChain[O, M]) PushAroundAdvisor(advisor api.Advisor) *DefaultAroundChain[O, M] {
	callAroundAdvisor, ok := advisor.(api.CallAroundAdvisor[O, M])
	if ok {
		d.callAroundAdvisors = append(d.callAroundAdvisors, callAroundAdvisor)
	}
	streamAroundAdvisor, ok := advisor.(api.StreamAroundAdvisor[O, M])
	if ok {
		d.streamAroundAdvisors = append(d.streamAroundAdvisors, streamAroundAdvisor)
	}
	return d
}

func (d *DefaultAroundChain[O, M]) popCallAroundAdvisor() (api.CallAroundAdvisor[O, M], error) {
	if len(d.callAroundAdvisors) == 0 {
		return nil, ErrorChainNoAroundAdvisor
	}
	rv := d.callAroundAdvisors[len(d.callAroundAdvisors)-1]
	d.callAroundAdvisors = d.callAroundAdvisors[:len(d.callAroundAdvisors)-1]
	return rv, nil
}

func (d *DefaultAroundChain[O, M]) popStreamAroundAdvisor() (api.StreamAroundAdvisor[O, M], error) {
	if len(d.streamAroundAdvisors) == 0 {
		return nil, ErrorChainNoAroundAdvisor
	}
	rv := d.streamAroundAdvisors[len(d.streamAroundAdvisors)-1]
	d.streamAroundAdvisors = d.streamAroundAdvisors[:len(d.streamAroundAdvisors)-1]
	return rv, nil
}

func (d *DefaultAroundChain[O, M]) NextAroundCall(ctx *api.Context[O, M]) error {
	advisor, err := d.popCallAroundAdvisor()
	if err != nil {
		return err
	}
	return advisor.AroundCall(ctx, d)
}

func (d *DefaultAroundChain[O, M]) NextAroundStream(ctx *api.Context[O, M]) error {
	advisor, err := d.popStreamAroundAdvisor()
	if err != nil {
		return err
	}
	return advisor.AroundStream(ctx, d)
}

func (d *DefaultAroundChain[O, M]) Clone() *DefaultAroundChain[O, M] {
	newChain := NewDefaultAroundChain[O, M]()
	newChain.callAroundAdvisors = make([]api.CallAroundAdvisor[O, M], len(d.callAroundAdvisors))
	newChain.streamAroundAdvisors = make([]api.StreamAroundAdvisor[O, M], len(d.streamAroundAdvisors))

	copy(newChain.callAroundAdvisors, d.callAroundAdvisors)
	copy(newChain.streamAroundAdvisors, d.streamAroundAdvisors)

	return newChain
}
