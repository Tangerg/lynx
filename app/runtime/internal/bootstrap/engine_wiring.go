package bootstrap

import (
	history "github.com/Tangerg/lynx/chathistory"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
)

type messageEnvironment struct {
	history      history.Store
	conversation *conversation.Messages
}

func prepareEngineConfig(cfg Config) (agentexec.Config, messageEnvironment) {
	ecfg := cfg.Engine
	ecfg.ChildSessionStore = newChildSessionStore(cfg.SessionStore)
	ecfg.ProcessStore = cfg.ProcessStore
	ecfg.Provider = cfg.Provider
	if ecfg.Todos == nil {
		ecfg.Todos = cfg.TodoStore
	}
	// Guard the concrete-nil before it lands in an interface field: a typed-nil
	// offloader would read as non-nil and drive the eviction middleware into a
	// nil-pointer Offload. Threshold rides along only when a store is present.
	if cfg.ToolResultStore != nil {
		ecfg.ToolResultStore = cfg.ToolResultStore
		ecfg.ToolResultThreshold = cfg.ToolResultThreshold
	}
	messages := buildMessageEnvironment(&ecfg)
	return ecfg, messages
}

func buildMessageEnvironment(ecfg *agentexec.Config) messageEnvironment {
	historyStore := ecfg.HistoryStore
	if historyStore == nil {
		historyStore = history.NewInMemoryStore()
		ecfg.HistoryStore = historyStore
	}
	return messageEnvironment{
		history:      historyStore,
		conversation: conversation.NewMessages(historyStore),
	}
}

func attachToolEnvironment(ecfg *agentexec.Config, built toolset.Built) {
	if built.Resolver == nil {
		ecfg.ToolResolver = nil
		return
	}
	ecfg.ToolResolver = built.Resolver
}
