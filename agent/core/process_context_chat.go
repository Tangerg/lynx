package core

import (
	"context"
	"errors"
	"iter"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

// Chat returns a target chat client scoped to this process. Platform and
// process guardrails wrap the shared client without mutating it. When a session
// or process ID exists, every call is also bound to the corresponding
// chathistory conversation through context scope.
func (pc *ProcessContext) Chat() (*chatclient.Client, error) {
	if pc == nil || pc.chatClient == nil {
		return nil, errors.New("agent.ProcessContext.Chat: no ChatClient configured on the platform")
	}

	callMiddleware := make([]chat.CallMiddleware, 0, 1)
	streamMiddleware := make([]chat.StreamMiddleware, 0, 1)
	if id := pc.conversationID(); id != "" {
		callMiddleware = append(callMiddleware, bindCallConversation(id))
		streamMiddleware = append(streamMiddleware, bindStreamConversation(id))
	}
	if !pc.guardrails.Empty() {
		callMiddleware = append(callMiddleware, pc.guardrails.CallMiddlewares...)
		streamMiddleware = append(streamMiddleware, pc.guardrails.StreamMiddlewares...)
	}
	if len(callMiddleware) == 0 && len(streamMiddleware) == 0 {
		return pc.chatClient, nil
	}

	return chatclient.New(
		pc.chatClient,
		chatclient.WithStreamer(pc.chatClient),
		chatclient.WithCallMiddleware(callMiddleware...),
		chatclient.WithStreamMiddleware(streamMiddleware...),
	)
}

func bindCallConversation(id string) chat.CallMiddleware {
	return func(next chat.Model) chat.Model {
		return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
			return next.Call(chathistory.WithConversationID(ctx, id), request)
		})
	}
}

func bindStreamConversation(id string) chat.StreamMiddleware {
	return func(next chat.Streamer) chat.Streamer {
		return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
			return next.Stream(chathistory.WithConversationID(ctx, id), request)
		})
	}
}

// conversationID returns the process history partition. A session ID takes
// precedence over the process ID.
func (pc *ProcessContext) conversationID() string {
	var processID string
	if pc.Process != nil {
		processID = pc.Process.ID()
	}
	return ConversationID(pc.Options, processID)
}
