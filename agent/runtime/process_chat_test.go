package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
)

type requestContextKey struct{}

func TestProcessChatAppliesConfiguredCallMiddleware(t *testing.T) {
	const requestScope = "request-1"
	middleware := func(next chat.Model) chat.Model {
		return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
			return next.Call(context.WithValue(ctx, requestContextKey{}, requestScope), request)
		})
	}
	model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		if got, _ := ctx.Value(requestContextKey{}).(string); got != requestScope {
			return nil, errors.New("model call did not receive middleware context")
		}
		message := chat.NewAssistantMessage(chat.NewTextPart("answer"))
		return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	})
	process := &Process{
		id: requestScope,
		options: &processOptions{chatMiddleware: &core.ChatMiddleware{
			CallMiddlewares: []chat.CallMiddleware{middleware},
		}, maxModelCalls: 3},
	}

	scoped, err := process.scopeChat(core.ChatCapability{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Model.Call(t.Context(), request); err != nil {
		t.Fatal(err)
	}
}

func TestProcessChatComposesProcessMiddlewareOutsideEngineMiddleware(t *testing.T) {
	type key struct{}
	bind := func(next chat.Model) chat.Model {
		return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
			return next.Call(context.WithValue(ctx, key{}, "bound"), request)
		})
	}
	requireBound := func(next chat.Model) chat.Model {
		return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
			if ctx.Value(key{}) != "bound" {
				return nil, errors.New("engine middleware ran before process context binding")
			}
			return next.Call(ctx, request)
		})
	}
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		message := chat.NewAssistantMessage(chat.NewTextPart("answer"))
		return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	})
	process := &Process{
		engine: &Engine{chatMiddleware: &core.ChatMiddleware{
			CallMiddlewares: []chat.CallMiddleware{requireBound},
		}},
		options: &processOptions{chatMiddleware: &core.ChatMiddleware{
			CallMiddlewares: []chat.CallMiddleware{bind},
		}},
	}

	scoped, err := process.scopeChat(core.ChatCapability{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Model.Call(t.Context(), request); err != nil {
		t.Fatal(err)
	}
}
