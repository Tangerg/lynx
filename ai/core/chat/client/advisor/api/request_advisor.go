package api

import (
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type RequestAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	Advisor
	AdviseRequest(ctx *Context[O, M]) error
}

func ExtractRequestAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](advisors []Advisor) []RequestAdvisor[O, M] {
	rv := make([]RequestAdvisor[O, M], 0, len(advisors))
	for _, advisor := range advisors {
		requestAdvisor, ok := advisor.(RequestAdvisor[O, M])
		if ok {
			rv = append(rv, requestAdvisor)
		}
	}
	return rv
}
