// Package utilitymodel provides the narrow, middleware-free model call used by
// auxiliary Runtime capabilities. These calls must not enter the interactive
// conversation history, tool loop, or guardrail pipeline.
package utilitymodel

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

// Resolver selects the current utility-role client for each call. Resolving at
// the boundary lets a role configuration change take effect without rebuilding
// the owning worker.
type Resolver func(context.Context) *chatclient.Client

// callTimeout bounds one auxiliary model request independently of an Agent Run.
const callTimeout = 2 * time.Minute

// Complete performs one synchronous, middleware-free prompt completion.
func Complete(ctx context.Context, client *chatclient.Client, systemPrompt, userPrompt string) (string, error) {
	if client == nil {
		return "", errors.New("utilitymodel: client is required")
	}
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	response, err := client.Call(callCtx, &chat.Request{Messages: []chat.Message{
		chat.NewSystemMessage(systemPrompt),
		chat.NewUserMessage(chat.NewTextPart(userPrompt)),
	}})
	if err != nil {
		return "", err
	}
	return response.Text(), nil
}
