package agentexec

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

func newAgentRuntime(config Config, resolver toolport.ToolResolver) (*agentruntime.Engine, error) {
	guardrails, err := newChatGuardrails(config)
	if err != nil {
		return nil, err
	}

	extensions := make([]core.Extension, 0, 1)
	if resolver != nil {
		extensions = append(extensions, resolver)
	}

	return agent.NewEngine(agentruntime.Config{
		Chat:         core.ChatCapability{Model: config.ChatClient, Streamer: config.ChatClient},
		Extensions:   extensions,
		Guardrails:   guardrails,
		ProcessStore: config.ProcessStore,
		AutoSnapshot: config.ProcessStore != nil,
		SessionStore: config.SessionStore,
	})
}

// newChatGuardrails composes the shared chat pipeline for every top-level
// turn and subtask. The tool loop stays outermost and the history middleware
// stays model-adjacent, so each loop round persists only the genuinely-new
// messages for that conversation id.
func newChatGuardrails(config Config) (*core.ChatGuardrails, error) {
	return newChatGuardrailsWithBeforeRound(config.HistoryStore, nil)
}

func newChatGuardrailsWithBeforeRound(
	historyStore history.Store,
	beforeRound func(context.Context) []chat.Message,
) (*core.ChatGuardrails, error) {
	guardrails, err := agentruntime.NewChatGuardrails(agentruntime.ChatGuardrailsConfig{
		HistoryStore: historyStore,
	})
	if err != nil || beforeRound == nil {
		return guardrails, err
	}
	var rounds atomic.Uint64
	continuationMessages := func(ctx context.Context) []chat.Message {
		if rounds.Add(1) == 1 {
			return nil
		}
		return beforeRound(ctx)
	}
	guardrails.CallMiddlewares = append([]chat.CallMiddleware{beforeRoundCall(continuationMessages)}, guardrails.CallMiddlewares...)
	guardrails.StreamMiddlewares = append([]chat.StreamMiddleware{beforeRoundStream(continuationMessages)}, guardrails.StreamMiddlewares...)
	return guardrails, nil
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
