package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
)

func TestChatMiddlewareEmpty(t *testing.T) {
	if !(*core.ChatMiddleware)(nil).Empty() {
		t.Fatal("nil ChatMiddleware must be empty")
	}
	if !(&core.ChatMiddleware{}).Empty() {
		t.Fatal("zero ChatMiddleware must be empty")
	}
	for name, middleware := range map[string]*core.ChatMiddleware{
		"call":   {CallMiddlewares: []chat.CallMiddleware{passthroughCall}},
		"stream": {StreamMiddlewares: []chat.StreamMiddleware{passthroughStream}},
	} {
		t.Run(name, func(t *testing.T) {
			if middleware.Empty() {
				t.Fatal("configured ChatMiddleware reported empty")
			}
		})
	}
}

func passthroughCall(next chat.Model) chat.Model         { return next }
func passthroughStream(next chat.Streamer) chat.Streamer { return next }
