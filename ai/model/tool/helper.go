package tool

import (
	stdContext "context"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/chat/request"
	"github.com/Tangerg/lynx/ai/model/chat/response"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

type Helper struct {
	registry *Registry
	invoker  *invoker
}

func NewHelper(cap ...int) *Helper {
	registry := NewRegistry(cap...)
	return &Helper{
		registry: registry,
		invoker:  newInvoker(registry),
	}
}

func (m *Helper) Registry() *Registry {
	return m.registry
}

func (m *Helper) ShouldReturnDirect(msgs []messages.Message) bool {
	if !messages.IsLastOfType(msgs, messages.Tool) {
		return false
	}

	message, _ := pkgSlices.Last(msgs)
	toolResponseMessage := message.(*messages.ToolResponseMessage)

	var returnDirect = true
	for _, toolResponse := range toolResponseMessage.ToolResponses() {
		t, ok := m.registry.Find(toolResponse.Name)
		if !ok {
			return false
		}
		returnDirect = returnDirect && t.Metadata().ReturnDirect()
	}

	return returnDirect
}

func (m *Helper) ShouldInvokeToolCalls(chatResponse *response.ChatResponse) (bool, error) {
	return m.invoker.shouldInvokeToolCalls(chatResponse)
}

func (m *Helper) InvokeToolCalls(ctx stdContext.Context, req *request.ChatRequest, resp *response.ChatResponse) (*InvokeResult, error) {
	return m.invoker.invoke(ctx, req, resp)
}
