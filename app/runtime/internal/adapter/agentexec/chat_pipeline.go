package agentexec

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

type chatMiddlewareBuilder func(
	history.Store,
	func(context.Context) []chat.Message,
) (*core.ChatMiddleware, error)

func newAgentRuntime(
	config Config,
	resolver ToolResolver,
) (*agentruntime.Engine, error) {
	extensions := make([]core.Extension, 0, 1)
	if resolver != nil {
		extensions = append(extensions, resolver)
	}

	return agent.NewEngine(agentruntime.Config{
		Chat:          core.ChatCapability{Model: config.ChatClient, Streamer: config.ChatClient},
		Extensions:    extensions,
		MaxModelCalls: turnMaxModelCalls,
	})
}

// newChatMiddleware composes the shared history pipeline for every top-level
// turn and subtask. Managed interaction remains framework-owned; the
// model-adjacent history middleware persists only genuinely new messages for
// the current conversation id.
func newChatMiddleware(config Config) (*core.ChatMiddleware, error) {
	return newChatMiddlewareWithBeforeRound(config.HistoryStore, nil)
}

func newChatMiddlewareWithBeforeRound(
	historyStore history.Store,
	beforeRound func(context.Context) []chat.Message,
) (*core.ChatMiddleware, error) {
	middleware, err := history.NewMiddleware(historyStore)
	if err != nil {
		return nil, fmt.Errorf("agentexec: build chat history middleware: %w", err)
	}
	middlewarePipeline := &core.ChatMiddleware{
		CallMiddlewares: []chat.CallMiddleware{middleware.Call},
		StreamMiddlewares: []chat.StreamMiddleware{
			middleware.Stream,
		},
	}
	if beforeRound == nil {
		return middlewarePipeline, nil
	}
	var rounds atomic.Uint64
	continuationMessages := func(ctx context.Context) []chat.Message {
		if rounds.Add(1) == 1 {
			return nil
		}
		return beforeRound(ctx)
	}
	middlewarePipeline.CallMiddlewares = append([]chat.CallMiddleware{beforeRoundCall(continuationMessages)}, middlewarePipeline.CallMiddlewares...)
	middlewarePipeline.StreamMiddlewares = append([]chat.StreamMiddleware{beforeRoundStream(continuationMessages)}, middlewarePipeline.StreamMiddlewares...)
	return middlewarePipeline, nil
}

// scopeHistory binds model calls to the application conversation selected for
// the active process. The Agent runtime sees only ordinary chat middleware:
// product conversation identity and child-history partitioning stay entirely
// inside this adapter.
func scopeHistory(base *core.ChatMiddleware, rootConversationID string) (*core.ChatMiddleware, error) {
	if rootConversationID != "" {
		if err := history.ValidateConversationID(rootConversationID); err != nil {
			return nil, fmt.Errorf("agentexec: scope chat history: %w", err)
		}
	}
	scoped := &core.ChatMiddleware{}
	if base != nil {
		*scoped = *base
		scoped.CallMiddlewares = slices.Clone(base.CallMiddlewares)
		scoped.StreamMiddlewares = slices.Clone(base.StreamMiddlewares)
	}
	scoped.CallMiddlewares = append(
		[]chat.CallMiddleware{bindHistoryCall(rootConversationID)},
		scoped.CallMiddlewares...,
	)
	scoped.StreamMiddlewares = append(
		[]chat.StreamMiddleware{bindHistoryStream(rootConversationID)},
		scoped.StreamMiddlewares...,
	)
	return scoped, nil
}

func bindHistoryCall(rootConversationID string) chat.CallMiddleware {
	return func(next chat.Model) chat.Model {
		return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
			bound, err := bindHistoryContext(ctx, rootConversationID)
			if err != nil {
				return nil, err
			}
			return next.Call(bound, request)
		})
	}
}

func bindHistoryStream(rootConversationID string) chat.StreamMiddleware {
	return func(next chat.Streamer) chat.Streamer {
		return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
			bound, err := bindHistoryContext(ctx, rootConversationID)
			if err != nil {
				return func(yield func(*chat.Response, error) bool) {
					yield(nil, err)
				}
			}
			return next.Stream(bound, request)
		})
	}
}

func bindHistoryContext(ctx context.Context, rootConversationID string) (context.Context, error) {
	process := core.ProcessViewFrom(ctx)
	if process == nil {
		return nil, errors.New("agentexec: scope chat history: process is missing from context")
	}
	conversationID := process.ID()
	if process.ParentID() == "" && rootConversationID != "" {
		conversationID = rootConversationID
	}
	if err := history.ValidateConversationID(conversationID); err != nil {
		return nil, fmt.Errorf("agentexec: scope chat history: process %q: %w", process.ID(), err)
	}
	return history.WithConversationID(ctx, conversationID), nil
}

func beforeRoundCall(source func(context.Context) []chat.Message) chat.CallMiddleware {
	return func(next chat.Model) chat.Model {
		return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
			prepared, err := appendBeforeRound(request, source(ctx))
			if err != nil {
				return nil, err
			}
			return next.Call(ctx, prepared)
		})
	}
}

func beforeRoundStream(source func(context.Context) []chat.Message) chat.StreamMiddleware {
	return func(next chat.Streamer) chat.Streamer {
		return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
			prepared, err := appendBeforeRound(request, source(ctx))
			if err != nil {
				return func(yield func(*chat.Response, error) bool) { yield(nil, err) }
			}
			return next.Stream(ctx, prepared)
		})
	}
}

func appendBeforeRound(request *chat.Request, messages []chat.Message) (*chat.Request, error) {
	if len(messages) == 0 {
		return request, nil
	}
	if request == nil {
		return nil, fmt.Errorf("agentexec: append before round: %w", chat.ErrInvalidRequest)
	}
	prepared := *request
	prepared.Messages = append(slices.Clone(request.Messages), messages...)
	if err := prepared.Validate(); err != nil {
		return nil, fmt.Errorf("agentexec: append before round: %w", err)
	}
	return &prepared, nil
}
