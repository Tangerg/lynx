package api

type StreamAroundAdvisor interface {
	Advisor
	AroundStream(ctx *Context, chain AroundAdvisorChain) error
}
