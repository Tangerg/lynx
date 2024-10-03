package api

type CallAroundAdvisor interface {
	Advisor
	AroundCall(ctx *Context, chain AroundAdvisorChain) error
}
