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
	"github.com/Tangerg/lynx/chathistory/inmemory"
)

// Engine is the Agent SDK execution boundary. It deploys the root and delegated Agents
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
	plan           PlanReader
	workdir        string
	userHome       string
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
	Summary      string
	Instructions string
	Reply        string
}

// SubagentProjection reads a delegated process directly rather than off an
// Agent event. Framework events carry lifecycle identity and nothing of the
// host's, so this is where Runtime turns a process id back into its own types —
// with the concrete delegateTaskInput in reach, instead of an any that every caller has
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
	if input, ok := core.Get[delegateTaskInput](blackboard, core.DefaultBindingName); ok {
		projection.Summary = input.Summary
		projection.Instructions = input.Instructions
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
		config.HistoryStore = inmemory.New()
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
		plan:                   config.Plan,
		workdir:                config.Workdir,
		userHome:               config.UserHome,
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
		delegationDeployment, err := agentRuntime.Deploy(ctx, engine.buildDelegatedAgent())
		if err != nil {
			return nil, fmt.Errorf("engine: deploy delegated agent: %w", err)
		}
		delegationTool, err := runtime.NewAgentTool[delegateTaskInput, string](agentRuntime, delegationDeployment)
		if err != nil {
			return nil, fmt.Errorf("engine: build delegation tool: %w", err)
		}
		resolver.UseDelegationTool(delegationTool)
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
