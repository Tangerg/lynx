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
	checkpoints  CheckpointReader
	buildID      string

	historyStore   history.Store
	chatMiddleware *core.ChatMiddleware
	knowledge      KnowledgeReader
	memory         AgentMemoryReader
	memorySearch   MemorySearcher
	todos          TodoReader
	workdir        string
	pricing        accounting.Pricing

	toolResultStore     toolResultOffloader
	toolResultThreshold int

	defaultProvider        string
	modelStreamIdleTimeout time.Duration
	chatMiddlewareBuilder  chatMiddlewareBuilder
}

// SubagentProjection is Runtime's own typed view of one delegated process: the
// task it was given and, once it finishes, the answer it produced.
type SubagentProjection struct {
	Description string
	Prompt      string
	Reply       string
}

// SubagentProjection reads a delegated process directly rather than off an
// Agent event. Framework events carry lifecycle identity and nothing of the
// host's, so this is where Runtime turns a process id back into its own types —
// with the concrete taskInput in reach, instead of an any that every caller has
// to re-interrogate.
//
// Valid while the engine still owns the process tree, which is true for the
// duration of any event this answers.
func (e *Engine) SubagentProjection(processID string) (SubagentProjection, bool) {
	if e == nil || e.runtime == nil {
		return SubagentProjection{}, false
	}
	process, ok := e.runtime.Process(processID)
	if !ok {
		return SubagentProjection{}, false
	}
	blackboard := process.Blackboard()

	projection := SubagentProjection{}
	if input, ok := core.Get[taskInput](blackboard, core.DefaultBindingName); ok {
		projection.Description = input.Description
		projection.Prompt = input.Prompt
	}
	// A delegated agent's goal produces the reply text itself, so the reply is
	// the last string it bound — not an arbitrary trailing object.
	projection.Reply, _ = core.Last[string](blackboard)
	return projection, true
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
	if config.Checkpoints != nil && config.BuildID == "" {
		return nil, errors.New("engine: BuildID is required when Checkpoints is configured")
	}
	if config.HistoryStore == nil {
		config.HistoryStore = history.NewInMemoryStore()
	}
	chatMiddleware, err := newChatMiddleware(config)
	if err != nil {
		return nil, err
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
		chatMiddleware:         chatMiddleware,
		todos:                  config.Todos,
		workdir:                config.Workdir,
		pricing:                config.Pricing,
		checkpoints:            config.Checkpoints,
		buildID:                config.BuildID,
		toolResultStore:        config.ToolResultStore,
		toolResultThreshold:    config.ToolResultThreshold,
		defaultProvider:        config.Provider,
		modelStreamIdleTimeout: llmIdleTimeout,
		chatMiddlewareBuilder:  newChatMiddlewareWithBeforeRound,
	}

	if resolver != nil {
		taskDeployment, err := agentRuntime.Deploy(ctx, engine.buildSubtaskAgent())
		if err != nil {
			return nil, fmt.Errorf("engine: deploy task agent: %w", err)
		}
		taskTool, err := runtime.NewAgentTool[taskInput, string](agentRuntime, taskDeployment)
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
