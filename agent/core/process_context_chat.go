package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	chatconversation "github.com/Tangerg/lynx/core/model/chat/conversation"
)

// Chat returns a fresh [chat.ClientRequest] cloned from the platform's
// shared [chat.Client], or nil when the platform was constructed
// without one — actions that expect LLM access should nil-check (or
// use [ChatWithActionTools] which surfaces a clear error).
//
// Platform-level [Guardrails] (when configured) are pre-installed on
// the returned request — every call / stream the action issues
// passes through the global logger / safeguard / quota middlewares
// before reaching the underlying model.
//
// The process's conversation id ([Session.ID], falling back to the
// process id) is stamped onto the request params under
// chat conversation ID so the history middleware auto-loads /
// persists the conversation history — see [conversationID].
func (pc *ProcessContext) Chat() *chat.ClientRequest {
	if pc.chatClient == nil {
		return nil
	}
	return pc.buildChatRequest(nil)
}

// ChatWithActionTools is the "ask the LLM with my action's tools"
// shortcut: a [chat.ClientRequest] pre-loaded with the action's
// resolved tools. Middleware (tool loop, history, etc.) comes from
// [Guardrails] — configured by the caller via [ProcessOptions].
//
// When the action declares no ToolGroups, the request still carries
// the configured guardrails (tool loop is in the guardrails chain,
// not constructed here).
//
// Errors when no ChatClient is configured or tool resolution fails.
func (pc *ProcessContext) ChatWithActionTools(ctx context.Context) (*chat.ClientRequest, error) {
	if pc.chatClient == nil {
		return nil, errors.New("agent.ProcessContext.ChatWithActionTools: no ChatClient configured on the platform")
	}
	tools, err := pc.ActionTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent.ProcessContext.ChatWithActionTools: %w", err)
	}
	return pc.buildChatRequest(tools), nil
}

// buildChatRequest composes the per-action chat request. Middleware
// (tool loop, history, etc.) comes from [Guardrails] — configured by
// the caller via [ProcessOptions.Guardrails] or the platform default.
func (pc *ProcessContext) buildChatRequest(tools []AgentTool) *chat.ClientRequest {
	req := pc.chatClient.Chat()

	mws := pc.guardrails.MiddlewareValues()
	if len(mws) > 0 {
		req = req.WithMiddlewares(mws...)
	}
	if len(tools) > 0 {
		req = req.WithTools(tools...)
	}
	if id := pc.conversationID(); id != "" {
		req = req.WithParams(map[string]any{chatconversation.IDKey: id})
	}
	return req
}

// conversationID is this process's chat history conversation key, stamped
// onto every chat request under chat conversation ID so the history
// middleware loads / saves history (and the tool loop parks
// interrupted rounds) keyed by it. The derivation rule lives in
// [ConversationID]. Returns "" only when neither a session nor a
// process is available, leaving the request unstamped.
func (pc *ProcessContext) conversationID() string {
	var processID string
	if pc.Process != nil {
		processID = pc.Process.ID()
	}
	return ConversationID(pc.Options, processID)
}
