package agentexec

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/agent/interaction"
)

// ErrToolInputRequired is the app2-owned control-flow identity used by Tool
// implementations. Its concrete framework representation stays in this adapter.
var ErrToolInputRequired = interaction.ErrToolInputRequired

// ToolInvocation is the smallest product-facing attribution needed by Runtime
// tools. Framework process, deployment and effect identities remain private.
type ToolInvocation struct {
	callID string
	name   string
}

func (invocation ToolInvocation) CallID() string { return invocation.callID }
func (invocation ToolInvocation) Name() string   { return invocation.name }

func ToolInvocationFromContext(ctx context.Context) (ToolInvocation, bool) {
	framework, ok := interaction.ToolInvocationFromContext(ctx)
	if !ok {
		return ToolInvocation{}, false
	}
	call := framework.ToolCall()
	return ToolInvocation{callID: call.ID, name: call.Name}, true
}

// ToolInputContinuation contains only Tool-owned state and its validated answer.
type ToolInputContinuation struct {
	state    json.RawMessage
	response json.RawMessage
}

func (continuation ToolInputContinuation) State() json.RawMessage {
	return bytes.Clone(continuation.state)
}

func (continuation ToolInputContinuation) Response() json.RawMessage {
	return bytes.Clone(continuation.response)
}

func ToolInputContinuationFromContext(ctx context.Context) (ToolInputContinuation, bool) {
	framework, ok := interaction.ToolInputContinuationFromContext(ctx)
	if !ok {
		return ToolInputContinuation{}, false
	}
	return ToolInputContinuation{
		state: framework.State(), response: framework.Response(),
	}, true
}

func RequireToolInput(prompt, responseSchema, state json.RawMessage) error {
	return interaction.RequireToolInput(prompt, responseSchema, state)
}

func AdvertiseTools(ctx context.Context, names ...string) error {
	return interaction.AdvertiseTools(ctx, names...)
}
