package toolloop

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	chatconversation "github.com/Tangerg/lynx/core/model/chat/conversation"
)

type failingParkStore struct {
	err    error
	writes int
}

func (*failingParkStore) Consume(context.Context, string) (*ParkState, error) {
	return nil, nil
}

func (s *failingParkStore) Write(context.Context, string, *ParkState) error {
	s.writes++
	return s.err
}

func TestParkWriteFailurePropagatesFromCallAndStream(t *testing.T) {
	want := errors.New("park storage unavailable")
	for _, stream := range []bool{false, true} {
		name := "call"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			model := newFakeChatModel(t)
			store := &failingParkStore{err: want}
			tool := mustNewCallable(t, "gated", false, func(context.Context, string) (string, error) {
				return "", interruptErr{}
			})
			callMW, streamMW := NewMiddleware(Config{ParkStore: store})
			req, err := chat.NewClientRequest(model)
			if err != nil {
				t.Fatalf("NewClientRequest: %v", err)
			}
			req.WithMessages(chat.NewUserMessage("seed")).
				WithTools(tool).
				WithParams(map[string]any{chatconversation.IDKey: "ses_1"})

			var got error
			if stream {
				model.streamYield = []*chat.Response{responseWithToolCall(t, "gated", `{}`)}
				req.WithStreamMiddlewares(streamMW)
				for _, err := range req.Stream().Response(t.Context()) {
					if err != nil {
						got = err
						break
					}
				}
			} else {
				model.respond = func(*chat.Request) (*chat.Response, error) {
					return responseWithToolCall(t, "gated", `{}`), nil
				}
				req.WithCallMiddlewares(callMW)
				_, got = req.Call().Response(t.Context())
			}
			if !errors.Is(got, want) {
				t.Fatalf("error = %v, want park write failure", got)
			}
			if store.writes != 1 {
				t.Fatalf("park writes = %d, want 1", store.writes)
			}
		})
	}
}
