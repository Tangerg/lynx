package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
)

func TestGuardrailsEmpty(t *testing.T) {
	if !(*core.Guardrails)(nil).Empty() {
		t.Fatal("nil Guardrails must be empty")
	}
	if !(&core.Guardrails{}).Empty() {
		t.Fatal("zero Guardrails must be empty")
	}
	for name, guardrails := range map[string]*core.Guardrails{
		"call":   {CallMiddlewares: []chat.CallMiddleware{passthroughCall}},
		"stream": {StreamMiddlewares: []chat.StreamMiddleware{passthroughStream}},
		"tools":  {MaxToolRounds: 2},
	} {
		t.Run(name, func(t *testing.T) {
			if guardrails.Empty() {
				t.Fatal("configured Guardrails reported empty")
			}
		})
	}
}

func passthroughCall(next chat.Model) chat.Model         { return next }
func passthroughStream(next chat.Streamer) chat.Streamer { return next }
