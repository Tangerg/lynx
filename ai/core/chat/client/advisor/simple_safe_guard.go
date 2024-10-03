package advisor

import (
	"github.com/samber/lo"

	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
)

var _ api.CallAroundAdvisor = (*SimpleSafeGuardAroundAdvisor)(nil)
var _ api.StreamAroundAdvisor = (*SimpleSafeGuardAroundAdvisor)(nil)

func NewSafeGuardAroundAdvisor(sensitiveWords []string) *SimpleSafeGuardAroundAdvisor {
	return &SimpleSafeGuardAroundAdvisor{sensitiveWords: sensitiveWords}
}

type SimpleSafeGuardAroundAdvisor struct {
	sensitiveWords []string
}

func (s *SimpleSafeGuardAroundAdvisor) Name() string {
	return "SimpleSafeGuardAroundAdvisor"
}

func (s *SimpleSafeGuardAroundAdvisor) AroundCall(ctx *api.Context, chain api.AroundAdvisorChain) error {
	if len(s.sensitiveWords) > 0 {
		if lo.Contains(s.sensitiveWords, ctx.Request.UserText()) {
			return ErrorSensitiveUserText
		}
	}
	return chain.NextAroundCall(ctx)
}

func (s *SimpleSafeGuardAroundAdvisor) AroundStream(ctx *api.Context, chain api.AroundAdvisorChain) error {
	if len(s.sensitiveWords) > 0 {
		if lo.Contains(s.sensitiveWords, ctx.Request.UserText()) {
			return ErrorSensitiveUserText
		}
	}
	return chain.NextAroundCall(ctx)
}
