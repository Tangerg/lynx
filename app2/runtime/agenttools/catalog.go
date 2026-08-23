// Package agenttools assembles Run-scoped executable capabilities. It owns
// filesystem confinement and human-input gates; agentexec receives only the
// frozen tool slice for one Run.
package agenttools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/skills"
	toolcontract "github.com/Tangerg/lynx/tool"
	"github.com/Tangerg/lynx/tools/fs"
	"github.com/Tangerg/lynx/tools/shell"
	skilltools "github.com/Tangerg/lynx/tools/skills"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type ApprovalPolicy interface {
	GetApprovalMode(context.Context) (protocol.ApprovalMode, error)
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

type Catalog struct {
	policy   ApprovalPolicy
	mcp      MCPGateway
	results  ToolResultReader
	goals    GoalGateway
	plans    PlanGateway
	userHome string
}

func New(policy ApprovalPolicy, mcp MCPGateway, results ToolResultReader, goals GoalGateway, plans PlanGateway, userHome string) (*Catalog, error) {
	if policy == nil || goals == nil || plans == nil || !filepath.IsAbs(userHome) {
		return nil, errors.New("agenttools: approval policy, goal and Plan gateways, and absolute user home are required")
	}
	return &Catalog{policy: policy, mcp: mcp, results: results, goals: goals, plans: plans, userHome: filepath.Clean(userHome)}, nil
}

func (catalog *Catalog) ForRun(ctx context.Context, scope agentexec.ToolScope) ([]agentexec.ExecutableTool, error) {
	if scope.SessionID == "" || scope.RunID == "" || !filepath.IsAbs(scope.Workspace) || scope.Facts == nil {
		return nil, errors.New("agenttools: complete run scope is required")
	}
	executor, err := newConfinedExecutor(scope.Workspace)
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

	sources := skills.Merge(
		skills.Dir(filepath.Join(scope.Workspace, ".agents", "skills")),
		skills.Dir(filepath.Join(scope.Workspace, ".claude", "skills")),
		skills.Dir(filepath.Join(catalog.userHome, ".agents", "skills")),
		skills.Dir(filepath.Join(catalog.userHome, ".codex", "skills")),
	)
	progressive, err := skilltools.NewTools(sources)
	if err != nil {
		return nil, fmt.Errorf("agenttools: skills: %w", err)
	}
	for _, executable := range progressive {
		values = append(values, scopedTool{tool: executable, safety: protocol.SafetyClassSafe})
	}
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

	mode, err := catalog.policy.GetApprovalMode(ctx)
	if err != nil {
		return nil, fmt.Errorf("agenttools: approval mode: %w", err)
	}
	result := make([]agentexec.ExecutableTool, 0, len(values))
	modelNames := make(map[string]struct{}, len(values))
	for _, value := range values {
		tool := value.tool
		if !value.intrinsicInput && !value.autoApproved && requiresApproval(mode, value.safety) {
			tool = &approvalTool{Tool: tool, runID: scope.RunID, safety: value.safety}
		}
		paths, ok, capabilityErr := toolcontract.Capability[mutationPaths](tool)
		if capabilityErr != nil {
			return nil, capabilityErr
		}
		if ok {
			tool = &pathGuardTool{Tool: tool, paths: paths, root: executor.root}
		}
		if value.safety != protocol.SafetyClassSafe {
			tool = &planModeGate{Tool: tool, plans: catalog.plans, sessionID: scope.SessionID}
		}
		name := tool.Definition().Name
		if _, duplicate := modelNames[name]; duplicate {
			return nil, fmt.Errorf("agenttools: duplicate model-visible tool name %q", name)
		}
		modelNames[name] = struct{}{}
		result = append(result, agentexec.ExecutableTool{
			Tool: tool, SafetyClass: value.safety, IntrinsicInput: value.intrinsicInput,
		})
	}
	return result, nil
}

type scopedTool struct {
	tool           toolcontract.Tool
	safety         protocol.SafetyClass
	intrinsicInput bool
	autoApproved   bool
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
		modelName := mcpModelName(descriptor.Server, descriptor.Name)
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
			tool: executable, safety: protocol.SafetyClassNetwork, autoApproved: approved,
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

var invalidMCPName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func mcpModelName(server, remote string) string {
	digest := sha256.Sum256([]byte(server + "\x00" + remote))
	stem := strings.Trim(invalidMCPName.ReplaceAllString(server+"_"+remote, "_"), "_")
	const suffixLength = 13 // "_" plus twelve hexadecimal characters.
	maximumStem := 64 - len("mcp_") - suffixLength
	if len(stem) > maximumStem {
		stem = stem[:maximumStem]
	}
	if stem == "" {
		stem = "tool"
	}
	return "mcp_" + stem + "_" + hex.EncodeToString(digest[:6])
}

func requiresApproval(mode protocol.ApprovalMode, safety protocol.SafetyClass) bool {
	switch mode {
	case protocol.ApprovalModeYolo:
		return false
	case protocol.ApprovalModeBalanced:
		return safety == protocol.SafetyClassExec
	case protocol.ApprovalModeSafe:
		return safety != protocol.SafetyClassSafe
	default:
		return true
	}
}

type approvalTool struct {
	toolcontract.Tool
	runID  string
	safety protocol.SafetyClass
}

func (tool *approvalTool) Unwrap() toolcontract.Tool { return tool.Tool }

type approvalContinuation struct {
	Arguments string `json:"arguments"`
}

type approvalResponse struct {
	Decision   protocol.ApprovalDecision `json:"decision"`
	EditedArgs map[string]any            `json:"editedArgs,omitempty"`
	Reason     string                    `json:"reason,omitempty"`
}

var approvalResponseSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"decision":{"type":"string","enum":["approve","deny"]},"editedArgs":{"type":"object"},"reason":{"type":"string"}},"required":["decision"]}`)

func (tool *approvalTool) Call(ctx context.Context, arguments string) (string, error) {
	if continuation, resumed := interaction.ToolInputContinuationFromContext(ctx); resumed {
		var state approvalContinuation
		var response approvalResponse
		if err := json.Unmarshal(continuation.State(), &state); err != nil {
			return "", fmt.Errorf("agenttools: restore approval state: %w", err)
		}
		if err := json.Unmarshal(continuation.Response(), &response); err != nil {
			return "", fmt.Errorf("agenttools: decode approval response: %w", err)
		}
		if response.Decision == protocol.ApprovalDeny {
			return "", fmt.Errorf("tool denied by user: %s", strings.TrimSpace(response.Reason))
		}
		if response.Decision != protocol.ApprovalApprove {
			return "", errors.New("agenttools: unknown approval decision")
		}
		arguments = state.Arguments
		if response.EditedArgs != nil {
			encoded, err := json.Marshal(response.EditedArgs)
			if err != nil {
				return "", err
			}
			arguments = string(encoded)
		}
		return tool.Tool.Call(ctx, arguments)
	}
	invocation, ok := interaction.ToolInvocationFromContext(ctx)
	if !ok {
		return "", errors.New("agenttools: approval tool called outside an Interaction")
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
		return "", fmt.Errorf("agenttools: decode approval arguments: %w", err)
	}
	risk := protocol.ApprovalRiskMedium
	if tool.safety == protocol.SafetyClassExec || tool.safety == protocol.SafetyClassNetwork {
		risk = protocol.ApprovalRiskHigh
	}
	prompt, err := json.Marshal(agentexec.ToolInputPrompt{
		Kind: "approval", ItemID: itemID(tool.runID, invocation.ToolCall().ID),
		Tool: &agentexec.ToolInputInvocation{Name: invocation.ToolCall().Name, Arguments: decoded},
		SafetyClass: tool.safety, Risk: risk, Reason: approvalReason(tool.safety), Rememberable: false,
	})
	if err != nil {
		return "", err
	}
	state, err := json.Marshal(approvalContinuation{Arguments: arguments})
	if err != nil {
		return "", err
	}
	return "", interaction.RequireToolInput(prompt, approvalResponseSchema, state)
}

func approvalReason(safety protocol.SafetyClass) string {
	switch safety {
	case protocol.SafetyClassWrite:
		return "This tool changes workspace files."
	case protocol.SafetyClassExec:
		return "This tool executes a local command."
	case protocol.SafetyClassNetwork:
		return "This tool accesses the network."
	default:
		return "This tool requires confirmation."
	}
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
	paths mutationPaths
	root  string
}

func (tool *pathGuardTool) Unwrap() toolcontract.Tool { return tool.Tool }

func (tool *pathGuardTool) Call(ctx context.Context, arguments string) (string, error) {
	paths, err := tool.paths.MutationPaths(arguments)
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		if _, err := confine(tool.root, path); err != nil {
			return "", err
		}
	}
	return tool.Tool.Call(ctx, arguments)
}

type confinedExecutor struct {
	root     string
	delegate *fs.LocalExecutor
}

func newConfinedExecutor(root string) (*confinedExecutor, error) {
	physical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("agenttools: resolve workspace: %w", err)
	}
	physical, err = filepath.Abs(physical)
	if err != nil {
		return nil, err
	}
	return &confinedExecutor{root: physical, delegate: fs.NewLocalExecutor(physical)}, nil
}

func (executor *confinedExecutor) Read(ctx context.Context, input fs.ReadInput) (fs.ReadOutput, error) {
	path, err := confine(executor.root, input.Path); if err != nil { return fs.ReadOutput{}, err }; input.Path = path
	return executor.delegate.Read(ctx, input)
}
func (executor *confinedExecutor) Write(ctx context.Context, input fs.WriteInput) (fs.WriteOutput, error) {
	path, err := confine(executor.root, input.Path); if err != nil { return fs.WriteOutput{}, err }; input.Path = path
	return executor.delegate.Write(ctx, input)
}
func (executor *confinedExecutor) Edit(ctx context.Context, input fs.EditInput) (fs.EditOutput, error) {
	path, err := confine(executor.root, input.Path); if err != nil { return fs.EditOutput{}, err }; input.Path = path
	return executor.delegate.Edit(ctx, input)
}
func (executor *confinedExecutor) ApplyPatch(ctx context.Context, input fs.ApplyPatchInput) (fs.ApplyPatchOutput, error) {
	return executor.delegate.ApplyPatch(ctx, input)
}
func (executor *confinedExecutor) Glob(ctx context.Context, input fs.GlobInput) (fs.GlobOutput, error) {
	if filepath.IsAbs(input.Pattern) || containsParent(input.Pattern) { return fs.GlobOutput{}, protocol.ErrPathOutsideRoot }
	if input.Root != "" { path, err := confine(executor.root, input.Root); if err != nil { return fs.GlobOutput{}, err }; input.Root = path }
	return executor.delegate.Glob(ctx, input)
}
func (executor *confinedExecutor) Grep(ctx context.Context, input fs.GrepInput) (fs.GrepOutput, error) {
	if input.Root != "" { path, err := confine(executor.root, input.Root); if err != nil { return fs.GrepOutput{}, err }; input.Root = path }
	return executor.delegate.Grep(ctx, input)
}

func confine(root, path string) (string, error) {
	if strings.TrimSpace(path) == "" || containsParent(path) {
		return "", protocol.ErrPathOutsideRoot
	}
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(root, filepath.Clean(path))
		if err != nil || escapes(relative) { return "", protocol.ErrPathOutsideRoot }
		path = relative
	}
	candidate := filepath.Join(root, filepath.Clean(path))
	parent := candidate
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			relative, relErr := filepath.Rel(root, resolved)
			if relErr != nil || escapes(relative) { return "", protocol.ErrPathOutsideRoot }
			break
		}
		if !errors.Is(err, os.ErrNotExist) { return "", err }
		next := filepath.Dir(parent)
		if next == parent { return "", protocol.ErrPathOutsideRoot }
		parent = next
	}
	return filepath.ToSlash(path), nil
}

func containsParent(path string) bool {
	for _, part := range strings.FieldsFunc(filepath.ToSlash(path), func(r rune) bool { return r == '/' }) {
		if part == ".." { return true }
	}
	return false
}

func escapes(relative string) bool {
	return relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
