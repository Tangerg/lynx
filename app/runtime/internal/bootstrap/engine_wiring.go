package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/history"

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
	ecfg.SessionStore = newChildSessionStore(cfg.SessionStore)
	ecfg.ProcessStore = cfg.ProcessStore
	ecfg.Provider = cfg.Provider
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

func wireEnginePorts(ecfg *agentexec.Config, cfg Config, messages messageEnvironment, resolveUtility func(context.Context) *chat.Client) {
	if ecfg.Steering == nil {
		ecfg.Steering = messages.conversation
	}
	wireMaintenancePorts(ecfg, cfg, messages.history, resolveUtility)
	if ecfg.Todos == nil {
		ecfg.Todos = cfg.TodoStore
	}
}

func attachToolEnvironment(ecfg *agentexec.Config, built toolset.Built) {
	ecfg.ToolResolver = built.Resolver
	ecfg.Tools = built.Tools
	ecfg.MCPStatusReader = built.MCPStatusReader
	ecfg.MCPToolCatalog = built.MCPToolCatalog
	ecfg.MCPConnectionCommands = built.MCPConnectionCommands
	ecfg.MCPRegistryCommands = built.MCPRegistryCommands
	ecfg.Closers = built.Closers
}
