package runtime

import (
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/chathistory"
	historymw "github.com/Tangerg/lynx/chathistory/middleware"
	"github.com/Tangerg/lynx/core/chat"
)

// ChatGuardrailsConfig assembles target chat middleware and bounded tool-loop
// policy for agent PromptRunner calls.
type ChatGuardrailsConfig struct {
	// HistoryStore persists complete chat exchanges. Nil selects an in-memory
	// store.
	HistoryStore chathistory.Store

	// MaxToolRounds bounds synchronous tool-loop model calls. Zero selects the
	// runner default; negative values are rejected.
	MaxToolRounds int
}

// BuildChatGuardrails builds history middleware and tool-loop limits without
// reintroducing executable tools or conversation IDs into chat.Request.
func BuildChatGuardrails(config ChatGuardrailsConfig) (*core.Guardrails, error) {
	if config.MaxToolRounds < 0 {
		return nil, fmt.Errorf("runtime.BuildChatGuardrails: MaxToolRounds must not be negative")
	}
	store := config.HistoryStore
	if store == nil {
		store = chathistory.NewInMemoryStore()
	}
	middleware, err := historymw.New(store)
	if err != nil {
		return nil, fmt.Errorf("runtime.BuildChatGuardrails: history: %w", err)
	}
	return &core.Guardrails{
		CallMiddlewares:   []chat.CallMiddleware{middleware.Call},
		StreamMiddlewares: []chat.StreamMiddleware{middleware.Stream},
		MaxToolRounds:     config.MaxToolRounds,
	}, nil
}
