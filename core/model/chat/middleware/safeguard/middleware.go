package safeguard

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/core/model/chat"
)

func (m *safeguardMiddleware) wrapCall(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		if m.opts.Scope.inspectsInput() {
			if term, hit := m.scanInputs(ctx, req); hit {
				m.opts.OnBlock(ctx, ScopeInput, term)
				return nil, blockError(ScopeInput, term)
			}
		}

		resp, err := next.Call(ctx, req)
		if err != nil {
			return resp, err
		}

		if term, hit := m.scanOutput(ctx, resp); hit {
			m.opts.OnBlock(ctx, ScopeOutput, term)
			return nil, blockError(ScopeOutput, term)
		}
		return resp, nil
	})
}

func (m *safeguardMiddleware) wrapStream(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			if m.opts.Scope.inspectsInput() {
				if term, hit := m.scanInputs(ctx, req); hit {
					m.opts.OnBlock(ctx, ScopeInput, term)
					yield(nil, blockError(ScopeInput, term))
					return
				}
			}

			for resp, err := range next.Stream(ctx, req) {
				if err != nil {
					if !yield(resp, err) {
						return
					}
					continue
				}

				if !yield(resp, nil) {
					return
				}

				if term, hit := m.scanOutput(ctx, resp); hit {
					m.opts.OnBlock(ctx, ScopeOutput, term)
					yield(nil, blockError(ScopeOutput, term))
					return
				}
			}
		}
	})
}
