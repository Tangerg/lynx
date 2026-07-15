package model_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/model"
)

func Example() {
	reasoning := int64(4)
	usage := model.Usage{
		PromptTokens:     20,
		CompletionTokens: 8,
		ReasoningTokens:  &reasoning,
	}
	rateLimit := model.RateLimit{RequestsRemaining: 99, TokensRemaining: 972}

	fmt.Println(usage.TotalTokens(), usage.HasReasoningTokens())
	fmt.Println(rateLimit.RequestsRemaining, rateLimit.TokensRemaining)
	// Output:
	// 28 true
	// 99 972
}
