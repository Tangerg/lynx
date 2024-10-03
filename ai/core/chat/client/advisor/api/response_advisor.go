package api

type ResponseAdvisor interface {
	Advisor
	AdviseCallResponse(ctx *Context) error
	AdviseStreamResponse(ctx *Context) error
}

func ExtractResponseAdvisor(advisors []Advisor) []ResponseAdvisor {
	rv := make([]ResponseAdvisor, 0, len(advisors))
	for _, advisor := range advisors {
		responseAdvisor, ok := advisor.(ResponseAdvisor)
		if ok {
			rv = append(rv, responseAdvisor)
		}
	}
	return rv
}
