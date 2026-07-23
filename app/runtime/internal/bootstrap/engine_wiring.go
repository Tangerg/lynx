package bootstrap

import (
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
)

type messageEnvironment struct {
	store        conversation.Store
	conversation *conversation.Messages
}

func prepareEngineConfig(cfg Config) (agentexec.Config, messageEnvironment, error) {
	ecfg := cfg.Engine
	ecfg.ChildSessionStore = agentexec.NewChildSessionStore(cfg.SessionStore)
	ecfg.ProcessStore = cfg.ProcessStore
	ecfg.Provider = cfg.Provider
	// These runtime-wide stores have one composition-root source of truth. The
	// engine receives only its read views; accepting a second value through
	// agentexec.Config would let the prompt and the corresponding application
	// use case silently observe different state.
	ecfg.Todos = cfg.TodoStore
	ecfg.Knowledge = cfg.KnowledgeStore
	// Guard the concrete-nil before it lands in an interface field: a typed-nil
	// offloader would read as non-nil and drive the eviction middleware into a
	// nil-pointer Stage. Threshold rides along only when a store is present.
	if cfg.ToolResultStore != nil {
		ecfg.ToolResultStore = cfg.ToolResultStore
		ecfg.ToolResultThreshold = cfg.ToolResultThreshold
	}
	messages, err := buildMessageEnvironment(&ecfg)
	return ecfg, messages, err
}

func buildMessageEnvironment(ecfg *agentexec.Config) (messageEnvironment, error) {
	if ecfg.HistoryStore == nil {
		return messageEnvironment{}, errors.New("runtime: Engine.HistoryStore is required")
	}
	store, ok := ecfg.HistoryStore.(conversation.Store)
	if !ok {
		return messageEnvironment{}, errors.New("runtime: Engine.HistoryStore must support atomic replace and count")
	}
	return messageEnvironment{
		store:        store,
		conversation: conversation.NewMessages(store),
	}, nil
}

func attachToolEnvironment(ecfg *agentexec.Config, built toolset.Built) {
	if built.Resolver == nil {
		ecfg.ToolResolver = nil
		return
	}
	ecfg.ToolResolver = built.Resolver
}
