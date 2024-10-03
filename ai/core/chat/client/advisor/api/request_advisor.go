package api

type RequestAdvisor interface {
	Advisor
	AdviseRequest(ctx *Context) error
}

func ExtractRequestAdvisor(advisors []Advisor) []RequestAdvisor {
	rv := make([]RequestAdvisor, 0, len(advisors))
	for _, advisor := range advisors {
		requestAdvisor, ok := advisor.(RequestAdvisor)
		if ok {
			rv = append(rv, requestAdvisor)
		}
	}
	return rv
}
