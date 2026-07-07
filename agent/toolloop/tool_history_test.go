package toolloop

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	chatconversation "github.com/Tangerg/lynx/core/model/chat/conversation"
	"github.com/Tangerg/lynx/core/model/chat/history"
	historymw "github.com/Tangerg/lynx/core/model/chat/middleware/history"
)

func TestHistory_SequentialMultiRoundTurn_ValidHistory(t *testing.T) {
	model := newFakeChatModel(t)
	model.streamRespond = func(req *chat.Request) []*chat.Response {
		switch countToolMsgs(req.Messages) {
		case 0:
			return []*chat.Response{toolCallResponseID(t, "call_1", "alpha")}
		case 1:
			return []*chat.Response{toolCallResponseID(t, "call_2", "beta")}
		default:
			return []*chat.Response{responseWithText("done")}
		}
	}
	alpha := mustNewCallable(t, "alpha", false, func(context.Context, string) (string, error) { return "a-ok", nil })
	beta := mustNewCallable(t, "beta", false, func(context.Context, string) (string, error) { return "b-ok", nil })

	store := history.NewInMemoryStore()
	historyCallMW, historyStreamMW, err := historymw.NewMiddleware(store)
	if err != nil {
		t.Fatal(err)
	}
	_, toolStreamMW := NewMiddleware()

	req, _ := chat.NewClientRequest(model)
	req.WithCallMiddlewares(historyCallMW).
		WithStreamMiddlewares(toolStreamMW, historyStreamMW).
		WithParams(map[string]any{chatconversation.IDKey: "c1"}).
		WithSystemPrompt("sys").
		WithUserPrompt("go").
		WithTools(alpha, beta)

	for _, e := range req.Stream().Response(context.Background()) {
		if e != nil {
			t.Fatalf("stream error: %v", e)
		}
	}

	stored, _ := store.Read(context.Background(), "c1")
	assertValidToolHistory(t, stored)
}
