package advisor

import (
	"github.com/samber/lo"

	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
)

var _ api.CallAroundAdvisor = (*SafeGuardAroundAdvisor)(nil)
var _ api.StreamAroundAdvisor = (*SafeGuardAroundAdvisor)(nil)

func NewSafeGuardAroundAdvisor(sensitiveWords []string) *SafeGuardAroundAdvisor {
	return &SafeGuardAroundAdvisor{sensitiveWords: sensitiveWords}
}

type SafeGuardAroundAdvisor struct {
	sensitiveWords []string
}

func (s *SafeGuardAroundAdvisor) Name() string {
	return "SafeGuardAroundAdvisor"
}

func (s *SafeGuardAroundAdvisor) AroundCall(ctx *api.Context, chain api.AroundAdvisorChain) error {
	if len(s.sensitiveWords) > 0 {
		if lo.Contains(s.sensitiveWords, ctx.Request.UserText()) {
			return ErrorSensitiveUserText
		}
	}
	return chain.NextAroundCall(ctx)
}

func (s *SafeGuardAroundAdvisor) AroundStream(ctx *api.Context, chain api.AroundAdvisorChain) error {
	if len(s.sensitiveWords) > 0 {
		if lo.Contains(s.sensitiveWords, ctx.Request.UserText()) {
			return ErrorSensitiveUserText
		}
	}
	return chain.NextAroundCall(ctx)
}
