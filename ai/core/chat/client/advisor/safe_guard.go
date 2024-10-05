package advisor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

func NewSafeGuardAroundAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](sensitiveWords []string) *SafeGuardAroundAdvisor[O, M] {
	return &SafeGuardAroundAdvisor[O, M]{sensitiveWords: sensitiveWords}
}

var _ api.CallAroundAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*SafeGuardAroundAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)
var _ api.StreamAroundAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*SafeGuardAroundAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type SafeGuardAroundAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	sensitiveWords []string
}

func (s *SafeGuardAroundAdvisor[O, M]) Name() string {
	return "SafeGuardAroundAdvisor"
}

func (s *SafeGuardAroundAdvisor[O, M]) AroundCall(ctx *api.Context[O, M], chain api.AroundAdvisorChain[O, M]) error {
	err := s.checkUserText(ctx.Request.UserText())
	if err != nil {
		return err
	}
	return chain.NextAroundCall(ctx)
}

func (s *SafeGuardAroundAdvisor[O, M]) AroundStream(ctx *api.Context[O, M], chain api.AroundAdvisorChain[O, M]) error {
	err := s.checkUserText(ctx.Request.UserText())
	if err != nil {
		return err
	}
	return chain.NextAroundCall(ctx)
}

func (s *SafeGuardAroundAdvisor[O, M]) checkUserText(userText string) error {
	if len(s.sensitiveWords) == 0 {
		return nil
	}

	var sensitiveWord string
	if lo.ContainsBy(s.sensitiveWords, func(word string) bool {
		if strings.Contains(userText, word) {
			sensitiveWord = word
			return true
		}
		return false
	}) {
		return errors.Join(ErrorSensitiveUserText, fmt.Errorf("the sensitiveWord is: %s", sensitiveWord))
	}
	return nil
}
