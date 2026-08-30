package safeguard

import (
	"context"
	"fmt"
	"iter"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
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

func (middleware *Middleware) inputError(ctx context.Context, request *chat.Request) error {
	block, err := middleware.scanInput(ctx, request)
	if err != nil {
		return err
	}
	if block != nil {
		return middleware.blocked(ctx, *block)
	}
	return nil
}

func (middleware *Middleware) outputError(ctx context.Context, response *chat.Response) error {
	block, err := middleware.scanOutput(ctx, response)
	if err != nil {
		return err
	}
	if block != nil {
		return middleware.blocked(ctx, *block)
	}
	return nil
}

// Call is a [chat.CallMiddleware]. Input is screened before the model runs;
// output is screened before a response becomes visible to the caller.
func (middleware *Middleware) Call(next chat.Model) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		if err := middleware.inputError(ctx, request); err != nil {
			return nil, err
		}

		response, err := next.Call(ctx, request)
		if err != nil {
			return response, err
		}
		if err := middleware.outputError(ctx, response); err != nil {
			return nil, err
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
			if err := middleware.inputError(ctx, request); err != nil {
				yield(nil, err)
				return
			}

			sequence := next.Stream(ctx, request)
			if sequence == nil {
				yield(nil, ErrNilStream)
				return
			}
			stream := safeguardStream{ctx: ctx, middleware: middleware, yield: yield}
			sequence(stream.consume)
		}
	})
}

type safeguardStream struct {
	ctx         context.Context
	middleware  *Middleware
	yield       func(*chat.Response, error) bool
	accumulator chat.ResponseAccumulator
	stopped     bool
}

func (s *safeguardStream) consume(chunk *chat.Response, streamErr error) bool {
	if s.stopped {
		return false
	}
	if streamErr != nil {
		return s.stop(chunk, streamErr)
	}
	if err := s.accumulator.Add(chunk); err != nil {
		return s.stop(nil, fmt.Errorf("safeguard: accumulate stream: %w", err))
	}
	if err := s.middleware.outputError(s.ctx, s.accumulator.Response()); err != nil {
		return s.stop(nil, err)
	}
	if !s.yield(chunk, nil) {
		s.stopped = true
		return false
	}
	return true
}

func (s *safeguardStream) stop(response *chat.Response, err error) bool {
	s.stopped = true
	s.yield(response, err)
	return false
}
