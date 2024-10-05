package api

import (
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type ResponseAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	Advisor
	AdviseCallResponse(ctx *Context[O, M]) error
	AdviseStreamResponse(ctx *Context[O, M]) error
}

func ExtractResponseAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](advisors []Advisor) []ResponseAdvisor[O, M] {
	rv := make([]ResponseAdvisor[O, M], 0, len(advisors))
	for _, advisor := range advisors {
		responseAdvisor, ok := advisor.(ResponseAdvisor[O, M])
		if ok {
			rv = append(rv, responseAdvisor)
		}
	}
	return rv
}
