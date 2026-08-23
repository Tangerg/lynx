// Package agenttools assembles Run-scoped executable capabilities. It owns
// filesystem confinement and the composed lifecycle/approval gates; agentexec
// receives only the frozen tool slice for one Run.
package agenttools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
	"github.com/Tangerg/lynx/tools/fs"
	"github.com/Tangerg/lynx/tools/shell"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/domain/approvalpolicy"
	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
	"github.com/Tangerg/lynx/app2/runtime/domain/mcpserver"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

type ApprovalPolicy interface {
	Mode(context.Context) (approvalpolicy.Mode, error)
	Decide(context.Context, approvalpolicy.Query) (approvalpolicy.Decision, bool, error)
	Remember(context.Context, approvalpolicy.Remember) error
}

type MCPGateway interface {
	Tools(context.Context, protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error)
	CallText(context.Context, string, string, map[string]any) (string, error)
	AutoApprovedTools(context.Context, string) ([]string, error)
}

type ToolResultReader interface {
	ReadToolResult(context.Context, string, string) (toolresult.Record, error)
}

type GoalGateway interface {
	Start(context.Context, protocol.StartGoalRequest) (*protocol.Goal, error)
	Get(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	IsOwnedRun(context.Context, string, string) (bool, error)
	Report(context.Context, string, string, string, string) (string, error)
}

type PlanGateway interface {
	Get(context.Context, protocol.GetPlanRequest) (*protocol.Plan, error)
	Replace(context.Context, string, []protocol.PlanStep) (*protocol.Plan, error)
	EnterMode(context.Context, string) (bool, error)
	ExitMode(context.Context, string) (bool, error)
	Mode(context.Context, string) (bool, error)
}

type LifecycleHooks interface {
	Evaluate(context.Context, lifecyclehook.Invocation) (lifecyclehook.Decision, error)
	EvaluateBestEffort(context.Context, lifecyclehook.Invocation) lifecyclehook.Decision
}

type Catalog struct {
	policy       ApprovalPolicy
	mcp          MCPGateway
	results      ToolResultReader
	goals        GoalGateway
	plans        PlanGateway
	skillGateway SkillGateway
	memory       MemoryGateway
	hooks        LifecycleHooks
}

type Config struct {
	Policy  ApprovalPolicy
	MCP     MCPGateway
	Results ToolResultReader
	Goals   GoalGateway
	Plans   PlanGateway
	Skills  SkillGateway
	Memory  MemoryGateway
	Hooks   LifecycleHooks
}

func New(config Config) (*Catalog, error) {
	if config.Policy == nil || config.Goals == nil || config.Plans == nil ||
		config.Skills == nil || config.Memory == nil || config.Hooks == nil {
		return nil, errors.New("agenttools: policy, domain gateways, and lifecycle hooks are required")
	}
	return &Catalog{
		policy: config.Policy, mcp: config.MCP, results: config.Results,
		goals: config.Goals, plans: config.Plans, skillGateway: config.Skills,
		memory: config.Memory, hooks: config.Hooks,
	}, nil
}

func (catalog *Catalog) ForRun(ctx context.Context, scope agentexec.ToolScope) ([]agentexec.ExecutableTool, error) {
	if scope.SessionID == "" || scope.RunID == "" || !filepath.IsAbs(scope.Workspace) || scope.Facts == nil {
		return nil, errors.New("agenttools: complete run scope is required")
	}
	executor, err := workspacefs.NewConfinedExecutor(scope.Workspace)
	if err != nil {
		return nil, err
	}
	values := []scopedTool{
		{tool: fs.NewReadTool(executor), safety: protocol.SafetyClassSafe},
		{tool: fs.NewGlobTool(executor), safety: protocol.SafetyClassSafe},
		{tool: fs.NewGrepTool(executor), safety: protocol.SafetyClassSafe},
		{tool: fs.NewApplyPatchTool(executor), safety: protocol.SafetyClassWrite},
		{tool: fs.NewEditTool(executor), safety: protocol.SafetyClassWrite},
		{tool: fs.NewWriteTool(executor), safety: protocol.SafetyClassWrite},
	}
	shellExecutor := shell.NewLocalExecutor()
	shellExecutor.Dir = scope.Workspace
	values = append(values, scopedTool{tool: shell.NewTool(shellExecutor), safety: protocol.SafetyClassExec})

	skillValues, err := catalog.skillTools(ctx, scope)
	if err != nil {
		return nil, err
	}
	values = append(values, skillValues...)
	memoryValues, err := catalog.memoryTools(scope)
	if err != nil {
		return nil, err
	}
	values = append(values, memoryValues...)
	question, err := newAskUser(scope.RunID)
	if err != nil {
		return nil, err
	}
	values = append(values, scopedTool{tool: question, safety: protocol.SafetyClassSafe, intrinsicInput: true})
	if catalog.results != nil {
		reader, err := newToolResultReader(catalog.results, scope.SessionID)
		if err != nil { return nil, err }
		values = append(values, scopedTool{tool: reader, safety: protocol.SafetyClassSafe})
	}
	if scope.IsRootRun {
		goalTools, err := catalog.goalTools(ctx, scope)
		if err != nil { return nil, err }
		values = append(values, goalTools...)
		planTools, err := catalog.planTools(scope)
		if err != nil { return nil, err }
		values = append(values, planTools...)
	}
	if catalog.mcp != nil {
		remote, err := catalog.remoteTools(ctx)
		if err != nil {
			return nil, err
		}
		values = append(values, remote...)
	}

	result := make([]agentexec.ExecutableTool, 0, len(values)+1)
	deferred := make([]agentexec.ExecutableTool, 0, len(values))
	modelNames := make(map[string]struct{}, len(values)+1)
	add := func(value scopedTool) error {
		binding, err := catalog.bindForRun(scope, executor, value)
		if err != nil {
			return err
		}
		name := binding.Tool.Definition().Name
		if _, duplicate := modelNames[name]; duplicate {
			return fmt.Errorf("agenttools: duplicate model-visible tool name %q", name)
		}
		modelNames[name] = struct{}{}
		result = append(result, binding)
		if binding.Deferred {
			deferred = append(deferred, binding)
		}
		return nil
	}
	for _, value := range values {
		if err := add(value); err != nil {
			return nil, err
		}
	}
	if len(deferred) > 0 {
		discovery, err := newToolDiscovery(deferred)
		if err != nil {
			return nil, err
		}
		if err := add(scopedTool{
			tool: discovery, safety: protocol.SafetyClassSafe,
		}); err != nil {
			return nil, err
		}
	}
	return result, nil
}

type scopedTool struct {
	tool           toolcontract.Tool
	safety         protocol.SafetyClass
	intrinsicInput bool
	autoApproved   bool
	deferred       bool
}

func (catalog *Catalog) bindForRun(
	scope agentexec.ToolScope,
	executor *workspacefs.ConfinedExecutor,
	value scopedTool,
) (agentexec.ExecutableTool, error) {
	executable := value.tool
	paths, ok, err := toolcontract.Capability[mutationPaths](executable)
	if err != nil {
		return agentexec.ExecutableTool{}, err
	}
	if ok {
		executable = &pathGuardTool{
			Tool: executable, paths: paths, executor: executor,
		}
	}
	if value.safety != protocol.SafetyClassSafe {
		executable = &planModeGate{
			Tool: executable, plans: catalog.plans, sessionID: scope.SessionID,
		}
	}
	executable = &lifecyclePolicyTool{
		Tool: executable, hooks: catalog.hooks, scope: scope,
		policy: catalog.policy, safety: value.safety,
		paths: paths, autoApproved: value.autoApproved,
		intrinsicInput: value.intrinsicInput,
	}
	return agentexec.ExecutableTool{
		Tool: executable, SafetyClass: value.safety,
		IntrinsicInput: value.intrinsicInput, Deferred: value.deferred,
	}, nil
}

func (catalog *Catalog) remoteTools(ctx context.Context) ([]scopedTool, error) {
	page, err := catalog.mcp.Tools(ctx, protocol.MCPListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("agenttools: list MCP tools: %w", err)
	}
	autoByServer := make(map[string]map[string]struct{})
	values := make([]scopedTool, 0, len(page.Data))
	modelNames := make(map[string]struct{}, len(page.Data))
	for _, descriptor := range page.Data {
		auto, found := autoByServer[descriptor.Server]
		if !found {
			names, err := catalog.mcp.AutoApprovedTools(ctx, descriptor.Server)
			if err != nil {
				return nil, fmt.Errorf("agenttools: MCP policy for %q: %w", descriptor.Server, err)
			}
			auto = make(map[string]struct{}, len(names))
			for _, name := range names {
				auto[name] = struct{}{}
			}
			autoByServer[descriptor.Server] = auto
		}
		modelName, err := mcpserver.ToolName(descriptor.Server, descriptor.Name)
		if err != nil {
			return nil, err
		}
		if _, duplicate := modelNames[modelName]; duplicate {
			return nil, fmt.Errorf("agenttools: duplicate MCP model name %q", modelName)
		}
		modelNames[modelName] = struct{}{}
		executable, err := newMCPTool(catalog.mcp, modelName, descriptor)
		if err != nil {
			return nil, err
		}
		_, approved := auto[descriptor.Name]
		values = append(values, scopedTool{
			tool: executable, safety: protocol.SafetyClassNetwork,
			autoApproved: approved, deferred: true,
		})
	}
	return values, nil
}

type mcpTool struct {
	definition chat.ToolDefinition
	gateway    MCPGateway
	server     string
	remoteName string
}

func newMCPTool(gateway MCPGateway, modelName string, descriptor protocol.MCPTool) (*mcpTool, error) {
	schema, err := json.Marshal(descriptor.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("agenttools: encode MCP schema %s/%s: %w", descriptor.Server, descriptor.Name, err)
	}
	definition := chat.ToolDefinition{Name: modelName, Description: descriptor.Description, InputSchema: schema}
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("agenttools: invalid MCP tool %s/%s: %w", descriptor.Server, descriptor.Name, err)
	}
	return &mcpTool{definition: definition, gateway: gateway, server: descriptor.Server, remoteName: descriptor.Name}, nil
}

func (tool *mcpTool) Definition() chat.ToolDefinition { return tool.definition.Clone() }

func (tool *mcpTool) Call(ctx context.Context, arguments string) (string, error) {
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return "", fmt.Errorf("agenttools: decode MCP arguments: %w", err)
	}
	if object == nil {
		return "", errors.New("agenttools: MCP arguments must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", errors.New("agenttools: MCP arguments contain trailing data")
		}
		return "", fmt.Errorf("agenttools: decode MCP arguments: %w", err)
	}
	return tool.gateway.CallText(ctx, tool.server, tool.remoteName, object)
}

type askUserRequest struct {
	Fields []protocol.QuestionField `json:"fields" jsonschema:"minItems=1,maxItems=3"`
}

type askUserResponse struct {
	Answers [][]string `json:"answers"`
}

var questionResponseSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"answers":{"type":"array","minItems":1,"items":{"type":"array","minItems":1,"items":{"type":"string"}}}},"required":["answers"]}`)

func newAskUser(runID string) (toolcontract.Tool, error) {
	return toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "ask_user", Description: "Ask the user one to three short questions when their choice is required to continue. The Run pauses durably and resumes with ordered answers.",
	}, func(ctx context.Context, request askUserRequest) (string, error) {
		if continuation, resumed := interaction.ToolInputContinuationFromContext(ctx); resumed {
			var response askUserResponse
			if err := json.Unmarshal(continuation.Response(), &response); err != nil {
				return "", err
			}
			encoded, err := json.Marshal(response)
			return string(encoded), err
		}
		question := protocol.Question{Fields: slices.Clone(request.Fields)}
		if err := question.ValidateWire(); err != nil {
			return "", fmt.Errorf("ask_user: %w", err)
		}
		invocation, ok := interaction.ToolInvocationFromContext(ctx)
		if !ok {
			return "", errors.New("ask_user: called outside an Interaction")
		}
		prompt, err := json.Marshal(agentexec.ToolInputPrompt{Kind: "question", ItemID: itemID(runID, invocation.ToolCall().ID), Question: &question})
		if err != nil {
			return "", err
		}
		state, _ := json.Marshal(struct{ CallID string `json:"callId"` }{CallID: invocation.ToolCall().ID})
		return "", interaction.RequireToolInput(prompt, questionResponseSchema, state)
	})
}

func itemID(runID, callID string) string {
	return agentexec.ToolItemID(runID, callID)
}

type readToolResultRequest struct {
	ResultID   string `json:"result_id" jsonschema:"minLength=1"`
	OffsetBytes int   `json:"offset_bytes,omitempty" jsonschema:"minimum=0"`
	LimitBytes  int   `json:"limit_bytes,omitempty" jsonschema:"minimum=1,maximum=20000"`
}

func newToolResultReader(reader ToolResultReader, sessionID string) (toolcontract.Tool, error) {
	return toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "read_tool_result",
		Description: "Read a byte window from a large Tool result previously replaced by a bounded preview. Use result_id from that preview; offset_bytes defaults to 0 and limit_bytes defaults to 20000.",
	}, func(ctx context.Context, request readToolResultRequest) (string, error) {
		record, err := reader.ReadToolResult(ctx, sessionID, request.ResultID)
		if err != nil { return "", err }
		limit := request.LimitBytes
		if limit == 0 { limit = 20_000 }
		if request.OffsetBytes < 0 || limit < 1 || limit > 20_000 {
			return "", errors.New("read_tool_result: invalid byte window")
		}
		if request.OffsetBytes >= len(record.Body) { return "", nil }
		start := request.OffsetBytes
		for start < len(record.Body) && !utf8.RuneStart(record.Body[start]) { start++ }
		end := min(len(record.Body), start+limit)
		for end > start && !utf8.ValidString(record.Body[start:end]) { end-- }
		window := record.Body[start:end]
		if end < len(record.Body) {
			window += fmt.Sprintf("\n\n[… continue with read_tool_result: {\"result_id\":%q,\"offset_bytes\":%d,\"limit_bytes\":%d}]", record.ID, end, limit)
		}
		return window, nil
	})
}

type mutationPaths interface {
	MutationPaths(string) ([]string, error)
}

type pathGuardTool struct {
	toolcontract.Tool
	paths    mutationPaths
	executor *workspacefs.ConfinedExecutor
}

func (tool *pathGuardTool) Unwrap() toolcontract.Tool { return tool.Tool }

func (tool *pathGuardTool) Call(ctx context.Context, arguments string) (string, error) {
	paths, err := tool.paths.MutationPaths(arguments)
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		if _, err := tool.executor.Path(path); err != nil {
			return "", err
		}
	}
	return tool.Tool.Call(ctx, arguments)
}
