// Package application is the Runtime composition-facing application facade.
// Its exported methods are deliberate operation capabilities, never a generic
// service locator.
package application

import (
	"context"
	"errors"
	"iter"
	"sync"

	"github.com/Tangerg/lynx/app2/runtime/boundarymeta"
	"github.com/Tangerg/lynx/app2/runtime/capabilityflow"
	"github.com/Tangerg/lynx/app2/runtime/codebaseflow"
	"github.com/Tangerg/lynx/app2/runtime/discovery"
	"github.com/Tangerg/lynx/app2/runtime/goalflow"
	"github.com/Tangerg/lynx/app2/runtime/interruptflow"
	"github.com/Tangerg/lynx/app2/runtime/memoryflow"
	"github.com/Tangerg/lynx/app2/runtime/mcpflow"
	"github.com/Tangerg/lynx/app2/runtime/operationsflow"
	"github.com/Tangerg/lynx/app2/runtime/planflow"
	"github.com/Tangerg/lynx/app2/runtime/providerflow"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
	"github.com/Tangerg/lynx/app2/runtime/runtimeevents"
	"github.com/Tangerg/lynx/app2/runtime/sessionflow"
	"github.com/Tangerg/lynx/app2/runtime/settingsflow"
	"github.com/Tangerg/lynx/app2/runtime/toolflow"
	"github.com/Tangerg/lynx/app2/runtime/workspaceflow"
)

type Runtime struct {
	discovery *discovery.Service
	sessions  *sessionflow.Service
	providers *providerflow.Service
	runs      *runflow.Service
	workspace *workspaceflow.Service
	settings  *settingsflow.Service
	interrupts *interruptflow.Service
	plans     *planflow.Service
	goals     *goalflow.Service
	goalDriver *goalflow.Driver
	mcp       *mcpflow.Service
	capability *capabilityflow.Service
	memory *memoryflow.Service
	codebase *codebaseflow.Service
	tools *toolflow.Service
	operations *operationsflow.Service
	events    *runtimeevents.Bus
	closeOnce sync.Once
}

type Config struct {
	Discovery *discovery.Service
	Sessions  *sessionflow.Service
	Providers *providerflow.Service
	Runs      *runflow.Service
	Workspace *workspaceflow.Service
	Settings *settingsflow.Service
	Events *runtimeevents.Bus
	Interrupts *interruptflow.Service
	Plans *planflow.Service
	Goals *goalflow.Service
	GoalDriver *goalflow.Driver
	MCP *mcpflow.Service
	Capability *capabilityflow.Service
	Memory *memoryflow.Service
	Codebase *codebaseflow.Service
	Tools *toolflow.Service
	Operations *operationsflow.Service
}

func New(config Config) (*Runtime, error) {
	if config.Discovery == nil || config.Sessions == nil || config.Providers == nil || config.Runs == nil || config.Workspace == nil || config.Settings == nil || config.Events == nil || config.Interrupts == nil || config.Plans == nil || config.Goals == nil || config.GoalDriver == nil || config.MCP == nil || config.Capability == nil || config.Memory == nil || config.Codebase == nil || config.Tools == nil || config.Operations == nil {
		return nil, errors.New("application: all required capability services must be supplied")
	}
	return &Runtime{
		discovery: config.Discovery, sessions: config.Sessions,
		providers: config.Providers, runs: config.Runs, workspace: config.Workspace,
		settings: config.Settings, interrupts: config.Interrupts, plans: config.Plans, goals: config.Goals, goalDriver: config.GoalDriver, mcp: config.MCP, capability: config.Capability, memory: config.Memory,
		codebase: config.Codebase, tools: config.Tools, operations: config.Operations, events: config.Events,
	}, nil
}

func (runtime *Runtime) Discover(ctx context.Context) (*protocol.DiscoverResponse, error) {
	return runtime.discovery.Discover(ctx)
}

func (runtime *Runtime) ListSessions(ctx context.Context, query protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
	return runtime.sessions.List(ctx, query)
}

func (runtime *Runtime) GetSession(ctx context.Context, sessionID string) (*protocol.Session, error) {
	return runtime.sessions.Get(ctx, sessionID)
}

func (runtime *Runtime) CreateSession(ctx context.Context, request protocol.CreateSessionRequest) (*protocol.Session, error) {
	value, err := runtime.sessions.Create(ctx, request)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSessionsChanged, SessionIDs: []string{value.ID}})
	}
	return value, err
}

func (runtime *Runtime) UpdateSession(ctx context.Context, request protocol.UpdateSessionRequest) (*protocol.Session, error) {
	value, err := runtime.sessions.Update(ctx, request)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSessionsChanged, SessionIDs: []string{value.ID}})
	}
	return value, err
}

func (runtime *Runtime) DeleteSession(ctx context.Context, sessionID string) error {
	hadGoal, suppressErr := runtime.goalDriver.SuppressSession(ctx, sessionID)
	if suppressErr != nil { return suppressErr }
	err := runtime.sessions.Delete(ctx, sessionID)
	runtime.goalDriver.ReleaseSession(sessionID, err == nil && hadGoal)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSessionsChanged, SessionIDs: []string{sessionID}})
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimePlanChanged, SessionIDs: []string{sessionID}})
	}
	return err
}

func (runtime *Runtime) GetSessionSnapshot(ctx context.Context, request protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error) {
	return runtime.sessions.Snapshot(ctx, request)
}

func (runtime *Runtime) ForkSession(ctx context.Context, request protocol.ForkSessionRequest) (*protocol.Session, error) {
	result, err := runtime.sessions.Fork(ctx, request)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSessionsChanged, SessionIDs: []string{result.Session.ID}})
		if result.PlanChanged { runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimePlanChanged, SessionIDs: []string{result.Session.ID}}) }
	}
	if err != nil { return nil, err }
	return result.Session, nil
}

func (runtime *Runtime) RollbackSession(ctx context.Context, request protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	hadGoal, suppressErr := runtime.goalDriver.SuppressSession(ctx, request.SessionID)
	if suppressErr != nil { return nil, suppressErr }
	result, err := runtime.sessions.Rollback(ctx, request)
	runtime.goalDriver.ReleaseSession(request.SessionID, err == nil && hadGoal && request.RestoreType != protocol.RestoreFiles)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSessionsChanged, SessionIDs: []string{request.SessionID}})
		if result.PlanChanged { runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimePlanChanged, SessionIDs: []string{request.SessionID}}) }
	}
	if err != nil { return nil, err }
	return result.Response, nil
}

func (runtime *Runtime) ExportSession(ctx context.Context, request protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
	return runtime.sessions.Export(ctx, request)
}

func (runtime *Runtime) ImportSession(ctx context.Context, request protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
	sessionID := request.Artifact.Session.ID
	hadGoal, suppressErr := runtime.goalDriver.SuppressSession(ctx, sessionID)
	if suppressErr != nil { return nil, suppressErr }
	result, err := runtime.sessions.Import(ctx, request)
	runtime.goalDriver.ReleaseSession(sessionID, err == nil && hadGoal)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSessionsChanged, SessionIDs: []string{result.Response.Session.ID}})
		if result.PlanChanged { runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimePlanChanged, SessionIDs: []string{result.Response.Session.ID}}) }
	}
	if err != nil { return nil, err }
	return result.Response, nil
}

func (runtime *Runtime) ResolveWorkspace(ctx context.Context, request protocol.ResolveWorkspaceRequest) (*protocol.WorkspaceInfo, error) {
	path := ""
	if request.Ref != nil {
		path = request.Ref.Path
	}
	return runtime.sessions.ResolveWorkspace(ctx, path)
}

func (runtime *Runtime) ListWorkspaces(ctx context.Context) (*protocol.Page[protocol.WorkspaceSummary], error) {
	return runtime.sessions.ListWorkspaces(ctx)
}

func (runtime *Runtime) ListProviders(ctx context.Context) (*protocol.Page[protocol.Provider], error) {
	return runtime.providers.List(ctx)
}

func (runtime *Runtime) UpdateProvider(ctx context.Context, request protocol.UpdateProviderRequest) (*protocol.Provider, error) {
	value, err := runtime.providers.Update(ctx, request)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeModelsChanged})
	}
	return value, err
}

func (runtime *Runtime) TestProvider(ctx context.Context, providerID string) (*protocol.ProviderTestResult, error) {
	return runtime.providers.Test(ctx, providerID)
}

func (runtime *Runtime) ListModels(ctx context.Context, request protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
	return runtime.providers.Models(ctx, request)
}

func (runtime *Runtime) GetUtilityRole(ctx context.Context) (*protocol.UtilityRole, error) {
	return runtime.providers.UtilityRole(ctx)
}

func (runtime *Runtime) SetUtilityRole(ctx context.Context, role protocol.UtilityRole) (*protocol.UtilityRole, error) {
	value, err := runtime.providers.SetUtilityRole(ctx, role)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeModelsChanged})
	}
	return value, err
}

func (runtime *Runtime) GetEmbeddingRole(ctx context.Context) (*protocol.EmbeddingRole, error) {
	return runtime.providers.EmbeddingRole(ctx)
}

func (runtime *Runtime) SetEmbeddingRole(ctx context.Context, role protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
	value, err := runtime.providers.SetEmbeddingRole(ctx, role)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeModelsChanged})
	}
	return value, err
}

func (runtime *Runtime) StartRun(ctx context.Context, request protocol.StartRunRequest) (
	*protocol.StartRunResponse,
	iter.Seq[protocol.RunEvent],
	error,
) {
	meta, _ := boundarymeta.RequestMetaFrom(ctx)
	response, events, err := runtime.runs.Start(ctx, runflow.StartCommand{Request: request, Meta: meta})
	return response, events, err
}

func (runtime *Runtime) ResumeRun(ctx context.Context, request protocol.ResumeRunRequest) (
	*protocol.ResumeRunResponse,
	iter.Seq[protocol.RunEvent],
	error,
) {
	return runtime.runs.ResumeWith(ctx, runflow.ResumeCommand{
		Request: request,
		BeforeLaunch: func(callbackCtx context.Context, runID string) error {
			return runtime.goalDriver.ObserveResumed(callbackCtx, runID)
		},
	})
}

func (runtime *Runtime) SteerRun(ctx context.Context, request protocol.SteerRunRequest) error {
	return runtime.runs.Steer(ctx, request)
}

func (runtime *Runtime) SubscribeRun(ctx context.Context, request protocol.SubscribeRunRequest) (
	*protocol.SubscribeRunResponse,
	iter.Seq[protocol.RunEvent],
	error,
) {
	return runtime.runs.Subscribe(ctx, request, boundarymeta.AfterEventIDFrom(ctx))
}

func (runtime *Runtime) CancelRun(ctx context.Context, request protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	meta, _ := boundarymeta.RequestMetaFrom(ctx)
	return runtime.runs.CancelWith(ctx, runflow.CancelCommand{Request: request, Meta: meta})
}

func (runtime *Runtime) GetRun(ctx context.Context, request protocol.GetRunRequest) (*protocol.RunRef, error) {
	return runtime.runs.Get(ctx, request.RunID)
}

func (runtime *Runtime) ListRuns(ctx context.Context, request protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
	return runtime.runs.List(ctx, request)
}

func (runtime *Runtime) ListItems(ctx context.Context, request protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	return runtime.runs.Items(ctx, request)
}

func (runtime *Runtime) ListInterrupts(ctx context.Context, request protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error) {
	return runtime.interrupts.List(ctx, request)
}

func (runtime *Runtime) GetPlan(ctx context.Context, request protocol.GetPlanRequest) (*protocol.Plan, error) {
	return runtime.plans.Get(ctx, request)
}

func (runtime *Runtime) ListWorkspaceFileChanges(ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	return runtime.workspace.Changes(ctx, request.Workspace)
}

func (runtime *Runtime) GetWorkspaceDiff(ctx context.Context, request protocol.GetDiffRequest) (*protocol.Diff, error) {
	return runtime.workspace.Diff(ctx, request)
}

func (runtime *Runtime) GetWorkspaceFileHead(ctx context.Context, request protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
	return runtime.workspace.Head(ctx, request)
}

func (runtime *Runtime) GrepWorkspace(ctx context.Context, request protocol.GrepRequest) (*protocol.GrepResult, error) {
	return runtime.workspace.Grep(ctx, request)
}

func (runtime *Runtime) ListWorkspaceFiles(ctx context.Context, request protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error) {
	return runtime.workspace.ListFiles(ctx, request)
}

func (runtime *Runtime) ReadWorkspaceFile(ctx context.Context, request protocol.ReadFileRequest) (*protocol.FileContent, error) {
	return runtime.workspace.ReadFile(ctx, request)
}

func (runtime *Runtime) SubscribeRuntime(ctx context.Context, request protocol.RuntimeSubscribeRequest) (
	*protocol.RuntimeSubscribeResponse,
	iter.Seq[protocol.RuntimeEvent],
	error,
) {
	return runtime.events.Subscribe(ctx, request)
}

func (runtime *Runtime) GetApprovalMode(ctx context.Context) (*protocol.ApprovalModeResult, error) {
	return runtime.settings.ApprovalMode(ctx)
}

func (runtime *Runtime) SetApprovalMode(ctx context.Context, request protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
	value, err := runtime.settings.SetApprovalMode(ctx, request)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeApprovalsChanged})
	}
	return value, err
}

func (runtime *Runtime) ListApprovalRules(ctx context.Context, request protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
	return runtime.settings.ApprovalRules(ctx, request)
}

func (runtime *Runtime) ForgetApprovalRule(ctx context.Context, request protocol.ForgetApprovalRuleRequest) error {
	err := runtime.settings.ForgetApprovalRule(ctx, request)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeApprovalsChanged})
	}
	return err
}

func (runtime *Runtime) ListSchedules(ctx context.Context, request protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
	return runtime.settings.ListSchedules(ctx, request)
}

func (runtime *Runtime) CreateSchedule(ctx context.Context, request protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
	value, err := runtime.settings.CreateSchedule(ctx, request)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSchedulesChanged, ScheduleIDs: []string{value.ID}})
	}
	return value, err
}

func (runtime *Runtime) UpdateSchedule(ctx context.Context, request protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
	value, err := runtime.settings.UpdateSchedule(ctx, request)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSchedulesChanged, ScheduleIDs: []string{value.ID}})
	}
	return value, err
}

func (runtime *Runtime) DeleteSchedule(ctx context.Context, request protocol.DeleteScheduleRequest) error {
	err := runtime.settings.DeleteSchedule(ctx, request)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSchedulesChanged, ScheduleIDs: []string{request.ID}})
	}
	return err
}

func (runtime *Runtime) RunScheduleNow(ctx context.Context, request protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error) {
	value, err := runtime.settings.RunNow(ctx, request)
	if err == nil {
		runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSchedulesChanged, ScheduleIDs: []string{request.ID}})
	}
	return value, err
}

func (runtime *Runtime) StartGoal(ctx context.Context, request protocol.StartGoalRequest) (*protocol.Goal, error) {
	return runtime.goals.Start(ctx, request)
}

func (runtime *Runtime) UpdateGoal(ctx context.Context, request protocol.UpdateGoalRequest) (*protocol.Goal, error) {
	previous, _, err := runtime.goals.Current(ctx, request.SessionID)
	if err != nil { return nil, err }
	value, err := runtime.goals.Update(ctx, request)
	if err == nil { runtime.goalDriver.CancelDetached(previous.ActiveRunID()) }
	return value, err
}

func (runtime *Runtime) ClearGoal(ctx context.Context, request protocol.GoalRequest) error {
	value, found, err := runtime.goals.Current(ctx, request.SessionID)
	if err != nil { return err }
	if err := runtime.goals.Clear(ctx, request); err != nil { return err }
	if found { runtime.goalDriver.CancelDetached(value.ActiveRunID()) }
	return nil
}

func (runtime *Runtime) GetGoal(ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
	return runtime.goals.Get(ctx, request)
}

func (runtime *Runtime) StopGoal(ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
	return runtime.goals.Stop(ctx, request)
}

func (runtime *Runtime) ResumeGoal(ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
	return runtime.goals.Resume(ctx, request)
}

func (runtime *Runtime) ListMCPServers(ctx context.Context) (*protocol.Page[protocol.MCPServer], error) {
	return runtime.mcp.List(ctx)
}

func (runtime *Runtime) CreateMCPServer(ctx context.Context, request protocol.MCPServerCandidate) (*protocol.MCPServer, error) {
	value, err := runtime.mcp.Create(ctx, request)
	if err == nil { runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeMCPChanged, ServerIDs: []string{value.Name}}) }
	return value, err
}

func (runtime *Runtime) UpdateMCPServer(ctx context.Context, request protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error) {
	value, err := runtime.mcp.Update(ctx, request)
	if err == nil { runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeMCPChanged, ServerIDs: []string{value.Name}}) }
	return value, err
}

func (runtime *Runtime) DeleteMCPServer(ctx context.Context, server string) error {
	err := runtime.mcp.Delete(ctx, server)
	if err == nil { runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeMCPChanged, ServerIDs: []string{server}}) }
	return err
}

func (runtime *Runtime) TestMCPServer(ctx context.Context, request protocol.MCPServerCandidate) (*protocol.MCPTestResult, error) {
	return runtime.mcp.Test(ctx, request)
}

func (runtime *Runtime) ListMCPTools(ctx context.Context, request protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error) {
	return runtime.mcp.Tools(ctx, request)
}

func (runtime *Runtime) ReconnectMCPServer(ctx context.Context, server string) error {
	err := runtime.mcp.Reconnect(ctx, server)
	if err == nil { runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeMCPChanged, ServerIDs: []string{server}}) }
	return err
}

func (runtime *Runtime) CreateMCPAuthorizationAttempt(ctx context.Context, server string) (*protocol.MCPAuthorizationAttempt, error) {
	value, err := runtime.mcp.CreateAuthorizationAttempt(ctx, server)
	if err == nil { runtime.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeMCPChanged, ServerIDs: []string{server}}) }
	return value, err
}

func (runtime *Runtime) GetMCPAuthorizationAttempt(ctx context.Context, id string) (*protocol.MCPAuthorizationAttempt, error) {
	return runtime.mcp.GetAuthorizationAttempt(ctx, id)
}

func (runtime *Runtime) ListDiscoveredSkills(ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.Skill], error) { return runtime.capability.DiscoveredSkills(ctx, request) }
func (runtime *Runtime) ListManagedSkills(ctx context.Context) (*protocol.Page[protocol.ManagedSkill], error) { return runtime.capability.ManagedSkills(ctx) }
func (runtime *Runtime) ArchiveSkill(ctx context.Context, request protocol.SkillNameRequest) error { err:=runtime.capability.SetSkillLifecycle(ctx,request,protocol.SkillLifecycleArchived); if err==nil{runtime.events.Publish(protocol.RuntimeEvent{Type:protocol.RuntimeSkillsChanged,Names:[]string{request.Name}})}; return err }
func (runtime *Runtime) RestoreSkill(ctx context.Context, request protocol.SkillNameRequest) error { err:=runtime.capability.SetSkillLifecycle(ctx,request,protocol.SkillLifecycleActive); if err==nil{runtime.events.Publish(protocol.RuntimeEvent{Type:protocol.RuntimeSkillsChanged,Names:[]string{request.Name}})}; return err }
func (runtime *Runtime) ListSkillProposals(ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) { return runtime.capability.SkillProposals(ctx,request) }
func (runtime *Runtime) ApproveSkillProposal(ctx context.Context, request protocol.SkillProposalRef) error { err:=runtime.capability.ApproveProposal(ctx,request);if err==nil{runtime.events.Publish(protocol.RuntimeEvent{Type:protocol.RuntimeSkillsChanged,Names:[]string{request.Name}})};return err }
func (runtime *Runtime) RejectSkillProposal(ctx context.Context, request protocol.SkillProposalRef) error { err:=runtime.capability.RejectProposal(ctx,request);if err==nil{runtime.events.Publish(protocol.RuntimeEvent{Type:protocol.RuntimeSkillsChanged,Names:[]string{request.Name}})};return err }
func (runtime *Runtime) ListRecipes(ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.Recipe], error) { return runtime.capability.Recipes(ctx,request) }
func (runtime *Runtime) ListAgentDocs(ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error) { return runtime.capability.AgentDocs(ctx,request) }
func (runtime *Runtime) ListHooks(ctx context.Context, request protocol.ListHooksRequest) (*protocol.HooksListResult, error) { return runtime.capability.ListHooks(ctx,request) }
func (runtime *Runtime) SetHookTrust(ctx context.Context, request protocol.SetHookTrustRequest) error { err:=runtime.capability.SetHookTrust(ctx,request);if err==nil{runtime.events.Publish(protocol.RuntimeEvent{Type:protocol.RuntimeHooksChanged})};return err }
func (runtime *Runtime) ListKnowledge(ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error) { return runtime.capability.ListKnowledge(ctx,request) }
func (runtime *Runtime) GetKnowledge(ctx context.Context, request protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error) { return runtime.capability.GetKnowledge(ctx,request) }
func (runtime *Runtime) UpdateKnowledge(ctx context.Context, request protocol.UpdateKnowledgeRequest) (*protocol.KnowledgeEntry, error) { value,err:=runtime.capability.UpdateKnowledge(ctx,request);if err==nil{runtime.events.Publish(protocol.RuntimeEvent{Type:protocol.RuntimeKnowledgeChanged})};return value,err }
func (runtime *Runtime) ListAgentMemory(ctx context.Context, request protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error) { return runtime.memory.List(ctx,request) }
func (runtime *Runtime) ReviewAgentMemory(ctx context.Context, request protocol.AgentMemoryReviewRequest) error { return runtime.memory.Review(ctx,request) }
func (runtime *Runtime) UpdateAgentMemory(ctx context.Context, request protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error) { return runtime.memory.Update(ctx,request) }
func (runtime *Runtime) DeleteAgentMemory(ctx context.Context, request protocol.AgentMemoryItemRequest) error { return runtime.memory.Delete(ctx,request.ID) }
func (runtime *Runtime) AddAgentMemory(ctx context.Context, request protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) { return runtime.memory.Add(ctx,request) }

func (runtime *Runtime) CodebaseSearch(ctx context.Context, request protocol.CodebaseSearchRequest) (*protocol.CodebaseSearchResult, error) { return runtime.codebase.Search(ctx,request) }
func (runtime *Runtime) CodebaseStatus(ctx context.Context, request protocol.CodebaseStatusRequest) (*protocol.CodebaseStatus, error) { return runtime.codebase.Status(ctx,request) }
func (runtime *Runtime) CodebaseReindex(ctx context.Context, request protocol.CodebaseReindexRequest) (*protocol.CodebaseReindexResponse, error) { return runtime.codebase.Reindex(ctx,request) }
func (runtime *Runtime) ListTools(ctx context.Context) (*protocol.Page[protocol.ToolSpec], error) { return runtime.tools.List(ctx) }
func (runtime *Runtime) InvokeTool(ctx context.Context, request protocol.InvokeToolRequest) (any,error) { return runtime.tools.Invoke(ctx,request) }
func (runtime *Runtime) SessionUsage(ctx context.Context, sessionID string) (*protocol.Usage,error) { return runtime.operations.SessionUsage(ctx,sessionID) }
func (runtime *Runtime) UsageSummary(ctx context.Context, request protocol.UsageSummaryRequest) (*protocol.UsageSummary,error) { return runtime.operations.UsageSummary(ctx,request) }
func (runtime *Runtime) CreateFeedback(ctx context.Context, request protocol.FeedbackRequest) error { return runtime.operations.Feedback(ctx,request) }

func (runtime *Runtime) Close() {
	if runtime == nil { return }
	runtime.closeOnce.Do(func() {
		runtime.settings.Close()
		runtime.goalDriver.Close()
		runtime.runs.Close()
		runtime.memory.Close()
		runtime.mcp.Close()
		runtime.codebase.Close()
		runtime.events.Close()
	})
}
