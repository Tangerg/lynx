package toolloop

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestToolLoop_InterruptThenResume(t *testing.T) {
	model := newFakeChatModel(t)
	modelCalls := 0
	model.streamRespond = func(*chat.Request) []*chat.Response {
		modelCalls++
		if modelCalls == 1 {
			return []*chat.Response{twoToolCallResponse("free", "gated")}
		}
		return []*chat.Response{responseWithText("done")}
	}

	var freeRuns, gatedRuns int
	approved := false
	freeTool := mustNewCallable(t, "free", false, func(context.Context, string) (string, error) {
		freeRuns++
		return "free-ok", nil
	})
	gatedTool := mustNewCallable(t, "gated", false, func(context.Context, string) (string, error) {
		if !approved {
			return "", interruptErr{}
		}
		gatedRuns++
		return "gated-ok", nil
	})

	_, streamMW := NewMiddleware()

	req1, _ := chat.NewClientRequest(model)
	req1.WithStreamMiddlewares(streamMW).WithMessages(chat.NewUserMessage("seed")).WithTools(freeTool, gatedTool)

	var (
		tail     []chat.Message
		firstErr error
	)
	for resp, e := range req1.Stream().Response(context.Background()) {
		if e != nil {
			firstErr = e
			break
		}
		if resp != nil && resp.Result != nil && resp.Result.Metadata != nil &&
			resp.Result.Metadata.FinishReason == FinishReasonInterrupt {
			tail = append(tail, resp.Result.AssistantMessage)
			if resp.Result.ToolMessage != nil {
				tail = append(tail, resp.Result.ToolMessage)
			}
		}
	}
	if !errors.As(firstErr, new(interruptErr)) {
		t.Fatalf("first run error = %v, want the tool's interruptErr", firstErr)
	}
	if modelCalls != 1 || freeRuns != 1 || gatedRuns != 0 {
		t.Fatalf("after interrupt: model=%d free=%d gated=%d, want 1/1/0", modelCalls, freeRuns, gatedRuns)
	}
	if len(tail) != 2 {
		t.Fatalf("interrupt tail = %d messages, want 2 (assistant + partial tool)", len(tail))
	}
	if tm, ok := tail[1].(*chat.ToolMessage); !ok || len(tm.ToolReturns) != 1 || tm.ToolReturns[0].Name != "free" {
		t.Fatalf("tail tool message = %+v, want one 'free' result", tail[1])
	}

	approved = true
	req2, _ := chat.NewClientRequest(model)
	req2.WithStreamMiddlewares(streamMW).WithMessages(tail...).WithTools(freeTool, gatedTool)

	_, finalText, err := collectStream(req2.Stream().Response(context.Background()))
	if err != nil {
		t.Fatalf("resume run error: %v", err)
	}
	if modelCalls != 2 {
		t.Fatalf("total model calls = %d, want 2 (round 1 NOT re-invoked on resume)", modelCalls)
	}
	if freeRuns != 1 {
		t.Fatalf("free ran %d times total, want 1 (completed call NOT re-executed)", freeRuns)
	}
	if gatedRuns != 1 {
		t.Fatalf("gated ran %d times, want 1 (executed once, on resume)", gatedRuns)
	}
	if finalText != "done" {
		t.Fatalf("final text = %q, want \"done\"", finalText)
	}
}
