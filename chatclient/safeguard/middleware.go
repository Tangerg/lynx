package safeguard

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/core/chat"
)

// Call is a [chat.CallMiddleware]. Input is screened before the model runs;
// output is screened before a response becomes visible to the caller.
func (m *Middleware) Call(next chat.Model) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		block, err := m.scanInput(ctx, request)
		if err != nil {
			return nil, err
		}
		if block != nil {
			return nil, m.blocked(ctx, *block)
		}

		response, err := next.Call(ctx, request)
		if err != nil {
			return response, err
		}
		block, err = m.scanOutput(ctx, response)
		if err != nil {
			return nil, err
		}
		if block != nil {
			return nil, m.blocked(ctx, *block)
		}
		return response, nil
	})
}

// Stream is a [chat.StreamMiddleware]. Output chunks are accumulated before
// screening so a term split across provider chunks is still detected. The
// chunk that completes an unsafe match is not yielded.
func (m *Middleware) Stream(next chat.Streamer) chat.Streamer {
	return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			block, err := m.scanInput(ctx, request)
			if err != nil {
				yield(nil, err)
				return
			}
			if block != nil {
				yield(nil, m.blocked(ctx, *block))
				return
			}

			sequence := next.Stream(ctx, request)
			if sequence == nil {
				yield(nil, ErrNilStream)
				return
			}
			var accumulator chat.ResponseAccumulator
			stopped := false
			sequence(func(chunk *chat.Response, streamErr error) bool {
				if stopped {
					return false
				}
				if streamErr != nil {
					stopped = true
					yield(chunk, streamErr)
					return false
				}
				if err := accumulator.Add(chunk); err != nil {
					stopped = true
					yield(nil, fmt.Errorf("safeguard: accumulate stream: %w", err))
					return false
				}
				block, err := m.scanOutput(ctx, accumulator.Response())
				if err != nil {
					stopped = true
					yield(nil, err)
					return false
				}
				if block != nil {
					stopped = true
					yield(nil, m.blocked(ctx, *block))
					return false
				}
				if !yield(chunk, nil) {
					stopped = true
					return false
				}
				return true
			})
		}
	})
}
