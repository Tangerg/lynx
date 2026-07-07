package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/model/chat"
	history "github.com/Tangerg/lynx/core/model/chat/history"
	historymw "github.com/Tangerg/lynx/core/model/chat/middleware/history"
)

// ToolLoopPolicy captures the run-time knobs for the turn-level tool loop.
//
// All fields are optional: zero values defer to toolloop defaults.
//
// BeforeRound runs only for continuation rounds (never the first call). It is
// intentionally callback-shaped so the caller can inject run-time concerns that
// should be shared by all tool-loop executions.
type ToolLoopPolicy struct {
	// FeedbackOnEmptyResponse makes the loop retry once when the model returns
	// neither text nor tool calls.
	FeedbackOnEmptyResponse bool

	// ParkStore enables transparent HITL resume across turns.
	ParkStore toolloop.ParkStore

	// LoopDetection, when non-nil, enables fixed-point guardrails before
	// the hard iteration cap.
	LoopDetection *toolloop.LoopDetectionConfig

	// BeforeRound runs on each continuation round before the next model call.
	BeforeRound func(context.Context) []chat.Message

	// MaxIterations caps tool-loop model calls. <=0 falls back to
	// toolloop.DefaultMaxIterations.
	MaxIterations int
}

// ChatGuardrailsConfig is the minimal input required to assemble a
// [core.Guardrails] instance for agent chat actions.
type ChatGuardrailsConfig struct {
	// HistoryStore backs chat history middleware. nil falls back to an in-memory
	// store.
	HistoryStore history.Store

	// ToolLoop carries the tool-loop policy.
	ToolLoop ToolLoopPolicy
}

// BuildChatGuardrails builds the default guardrails used by most chat actions: tool
// loop + history middleware.
func BuildChatGuardrails(cfg ChatGuardrailsConfig) (*core.Guardrails, error) {
	historyStore := cfg.HistoryStore
	if historyStore == nil {
		historyStore = history.NewInMemoryStore()
	}

	historyCallMW, historyStreamMW, err := historymw.NewMiddleware(historyStore)
	if err != nil {
		return nil, fmt.Errorf("runtime: build history middleware: %w", err)
	}

	toolCallMW, toolStreamMW := BuildToolLoop(cfg.ToolLoop)
	return &core.Guardrails{
		CallMiddlewares:   []chat.CallMiddleware{toolCallMW, historyCallMW},
		StreamMiddlewares: []chat.StreamMiddleware{toolStreamMW, historyStreamMW},
	}, nil
}

// BuildToolLoop returns the chat middleware pair implementing the runtime tool loop.
func BuildToolLoop(policy ToolLoopPolicy) (chat.CallMiddleware, chat.StreamMiddleware) {
	if policy.LoopDetection == nil {
		policy.LoopDetection = &toolloop.LoopDetectionConfig{}
	}

	callMW, streamMW := toolloop.NewMiddleware(toolloop.Config{
		MaxIterations:           policy.MaxIterations,
		FeedbackOnEmptyResponse: policy.FeedbackOnEmptyResponse,
		ParkStore:               policy.ParkStore,
		LoopDetection:           policy.LoopDetection,
		BeforeRound:             policy.BeforeRound,
	})

	return callMW, streamMW
}
