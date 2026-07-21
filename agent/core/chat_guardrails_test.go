package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
)

func TestGuardrailsEmpty(t *testing.T) {
	if !(*core.ChatGuardrails)(nil).Empty() {
		t.Fatal("nil ChatGuardrails must be empty")
	}
	if !(&core.ChatGuardrails{}).Empty() {
		t.Fatal("zero ChatGuardrails must be empty")
	}
	for name, guardrails := range map[string]*core.ChatGuardrails{
		"call":   {CallMiddlewares: []chat.CallMiddleware{passthroughCall}},
		"stream": {StreamMiddlewares: []chat.StreamMiddleware{passthroughStream}},
		"tools":  {MaxToolRounds: 2},
	} {
		t.Run(name, func(t *testing.T) {
			if guardrails.Empty() {
				t.Fatal("configured ChatGuardrails reported empty")
			}
		})
	}
}

func passthroughCall(next chat.Model) chat.Model         { return next }
func passthroughStream(next chat.Streamer) chat.Streamer { return next }
