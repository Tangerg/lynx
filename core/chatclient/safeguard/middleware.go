package safeguard

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

// MiddlewareConfig controls one immutable Middleware. A zero Scope defaults
// to ScopeBoth. OnBlock runs synchronously before a rejection is returned.
type MiddlewareConfig struct {
	Scope   Scope
	OnBlock func(context.Context, Block)
}

// Middleware screens model inputs and outputs at the model boundary.
type Middleware struct {
	matcher Matcher
	config  MiddlewareConfig
}

// NewMiddleware rejects nil matchers because silently disabling a safeguard
// would turn a configuration defect into a security boundary bypass.
func NewMiddleware(matcher Matcher, config MiddlewareConfig) (*Middleware, error) {
	if lo.IsNil(matcher) {
		return nil, fmt.Errorf("%w: matcher is nil", ErrInvalidMiddlewareConfig)
	}
	if config.Scope == "" {
		config.Scope = ScopeBoth
	}
	if !config.Scope.Valid() {
		return nil, fmt.Errorf("%w: unknown scope %q", ErrInvalidMiddlewareConfig, config.Scope)
	}
	return &Middleware{matcher: matcher, config: config}, nil
}

func (middleware *Middleware) blocked(ctx context.Context, block Block) error {
	if middleware.config.OnBlock != nil {
		middleware.config.OnBlock(ctx, block)
	}
	return &UnsafeError{Block: block}
}

func (middleware *Middleware) match(ctx context.Context, scope Scope, text string) (*Block, error) {
	if text == "" {
		return nil, nil
	}
	match, err := middleware.matcher.Match(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("safeguard: match %s content: %w", scope, err)
	}
	if !match.Found {
		return nil, nil
	}
	return &Block{Scope: scope, Term: match.Term}, nil
}

func (middleware *Middleware) scanInput(ctx context.Context, request *chat.Request) (*Block, error) {
	if !middleware.config.Scope.inspects(ScopeInput) || request == nil {
		return nil, nil
	}
	for index := range request.Messages {
		message := &request.Messages[index]
		if message.Role != chat.RoleSystem && message.Role != chat.RoleUser {
			continue
		}
		block, err := middleware.match(ctx, ScopeInput, message.Text())
		if err != nil || block != nil {
			return block, err
		}
	}
	return nil, nil
}

func (middleware *Middleware) scanOutput(ctx context.Context, response *chat.Response) (*Block, error) {
	if !middleware.config.Scope.inspects(ScopeOutput) || response == nil || response.Output == nil || response.Output.Message == nil {
		return nil, nil
	}
	return middleware.match(ctx, ScopeOutput, response.Output.Message.Text())
}

// Call is a [chat.CallMiddleware]. Input is screened before the model runs;
// output is screened before a response becomes visible to the caller.
func (middleware *Middleware) Call(next chat.Model) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		block, err := middleware.scanInput(ctx, request)
		if err != nil {
			return nil, err
		}
		if block != nil {
			return nil, middleware.blocked(ctx, *block)
		}

		response, err := next.Call(ctx, request)
		if err != nil {
			return response, err
		}
		block, err = middleware.scanOutput(ctx, response)
		if err != nil {
			return nil, err
		}
		if block != nil {
			return nil, middleware.blocked(ctx, *block)
		}
		return response, nil
	})
}

// Stream is a [chat.StreamMiddleware]. Output chunks are accumulated before
// screening so a term split across provider chunks is still detected. The
// chunk that completes an unsafe match is not yielded.
func (middleware *Middleware) Stream(next chat.Streamer) chat.Streamer {
	return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			block, err := middleware.scanInput(ctx, request)
			if err != nil {
				yield(nil, err)
				return
			}
			if block != nil {
				yield(nil, middleware.blocked(ctx, *block))
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
				block, err := middleware.scanOutput(ctx, accumulator.Response())
				if err != nil {
					stopped = true
					yield(nil, err)
					return false
				}
				if block != nil {
					stopped = true
					yield(nil, middleware.blocked(ctx, *block))
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
