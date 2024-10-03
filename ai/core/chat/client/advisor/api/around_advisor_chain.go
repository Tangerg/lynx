package api

type AroundAdvisorChain interface {
	NextAroundCall(ctx *Context) error
	NextAroundStream(ctx *Context) error
}
