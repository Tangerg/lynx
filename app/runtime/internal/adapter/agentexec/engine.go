package agentexec

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	history "github.com/Tangerg/lynx/chathistory"
)

// Engine is the Agent SDK execution boundary. It deploys the root/subtask Agent
// definitions and creates or restores their process trees. Prompt inputs stay
// here because the deployed actions consume them; application maintenance,
// integration control, tool catalogs, and resource shutdown are owned by their
// direct consumers and the composition Host.
type Engine struct {
	runtime      *runtime.Engine
	agent        *core.Agent
	dependencies *core.Dependencies

	historyStore history.Store
	knowledge    KnowledgeReader
	memory       AgentMemoryReader
	memorySearch MemorySearcher
	todos        TodoReader
	workdir      string
	pricing      accounting.Pricing

	toolResultStore     toolResultOffloader
	toolResultThreshold int

	defaultProvider        string
	modelStreamIdleTimeout time.Duration
	guardrailsBuilder      chatGuardrailsBuilder
}

// New constructs an execution engine. It rejects missing required dependencies
// and deployment failures synchronously.
func New(ctx context.Context, config Config) (*Engine, error) {
	if config.ChatClient == nil {
		return nil, errors.New("engine: ChatClient is required")
	}
	if config.BuildID != "" && !validBuildID(config.BuildID) {
		return nil, errors.New("engine: BuildID must use the format sha256:<64 lowercase hex characters>")
	}
	if config.ProcessStore != nil && config.BuildID == "" {
		return nil, errors.New("engine: BuildID is required when ProcessStore is configured")
	}
	if config.HistoryStore == nil {
		config.HistoryStore = history.NewInMemoryStore()
	}

	resolver := config.ToolResolver
	agentRuntime, err := newAgentRuntime(config, resolver)
	if err != nil {
		return nil, err
	}

	engine := &Engine{
		runtime:                agentRuntime,
		dependencies:           agentRuntime.Dependencies(),
		knowledge:              config.Knowledge,
		memory:                 config.AgentMemory,
		memorySearch:           config.MemorySearch,
		historyStore:           config.HistoryStore,
		todos:                  config.Todos,
		workdir:                config.Workdir,
		pricing:                config.Pricing,
		toolResultStore:        config.ToolResultStore,
		toolResultThreshold:    config.ToolResultThreshold,
		defaultProvider:        config.Provider,
		modelStreamIdleTimeout: llmIdleTimeout,
		guardrailsBuilder:      newChatGuardrailsWithBeforeRound,
	}

	if resolver != nil {
		if _, err := agentRuntime.Deploy(ctx, engine.buildSubtaskAgent()); err != nil {
			return nil, fmt.Errorf("engine: deploy task agent: %w", err)
		}
		taskTool, err := runtime.NewAgentTool[taskInput, string](agentRuntime, "task")
		if err != nil {
			return nil, fmt.Errorf("engine: build task tool: %w", err)
		}
		resolver.UseTaskTool(taskTool)
	}

	engine.agent = engine.buildTurnAgent()
	if _, err := agentRuntime.Deploy(ctx, engine.agent); err != nil {
		return nil, fmt.Errorf("engine: deploy turn agent: %w", err)
	}
	return engine, nil
}

func validBuildID(buildID string) bool {
	digest, ok := strings.CutPrefix(buildID, "sha256:")
	if !ok || len(digest) != sha256HexLength || digest != strings.ToLower(digest) {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

const sha256HexLength = 64
