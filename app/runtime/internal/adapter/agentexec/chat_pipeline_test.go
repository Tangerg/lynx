package agentexec

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

func TestScopeHistorySelectsRootConversationAndChildProcess(t *testing.T) {
	middleware, err := scopeHistory(nil, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		process core.ProcessView
		want    string
	}{
		{
			name:    "root uses product conversation",
			process: observedProcess{id: "root-process"},
			want:    "session-1",
		},
		{
			name:    "child owns isolated history",
			process: observedProcess{id: "child-process", parentID: "root-process"},
			want:    "child-process",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertConversation := func(ctx context.Context) error {
				got, ok := history.ConversationID(ctx)
				if !ok || got != test.want {
					t.Fatalf("conversation ID = %q/%v, want %q/true", got, ok, test.want)
				}
				return nil
			}
			model := middleware.CallMiddlewares[0](chat.ModelFunc(
				func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
					return nil, assertConversation(ctx)
				},
			))
			ctx := core.WithProcessView(t.Context(), test.process)
			if _, err := model.Call(ctx, nil); err != nil {
				t.Fatalf("Call: %v", err)
			}

			streamer := middleware.StreamMiddlewares[0](chat.StreamerFunc(
				func(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
					err := assertConversation(ctx)
					return func(yield func(*chat.Response, error) bool) {
						yield(nil, err)
					}
				},
			))
			for _, err := range streamer.Stream(ctx, nil) {
				if err != nil {
					t.Fatalf("Stream: %v", err)
				}
			}
		})
	}
}

func TestScopeHistoryRejectsMissingProcessContext(t *testing.T) {
	middleware, err := scopeHistory(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	model := middleware.CallMiddlewares[0](chat.ModelFunc(
		func(context.Context, *chat.Request) (*chat.Response, error) {
			t.Fatal("unscoped model was called")
			return nil, nil
		},
	))
	if _, err := model.Call(t.Context(), nil); err == nil {
		t.Fatal("Call without process context succeeded")
	}
}
