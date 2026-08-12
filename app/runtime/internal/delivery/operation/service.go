// Package operation owns the binding-neutral Runtime operation boundary shared
// by HTTP dispatch and the public embedded binding.
package operation

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// Service is the complete protocol projection implemented by delivery/server.
// It is private because protocol values are public while server implementation
// capabilities are not a client contract.
type Service interface {
	Discover(context.Context) (*protocol.DiscoverResponse, error)
	ListSessions(context.Context, protocol.PageQuery) (*protocol.Page[protocol.Session], error)
	GetSession(context.Context, string) (*protocol.Session, error)
	CreateSession(context.Context, protocol.CreateSessionRequest) (*protocol.Session, error)
	UpdateSession(context.Context, protocol.UpdateSessionRequest) (*protocol.Session, error)
	DeleteSession(context.Context, string) error
	ForkSession(context.Context, protocol.ForkSessionRequest) (*protocol.Session, error)
	RollbackSession(context.Context, protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error)
	ExportSession(context.Context, protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error)
	ImportSession(context.Context, protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error)
	StartRun(context.Context, protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error)
	ResumeRun(context.Context, protocol.ResumeRunRequest) (*protocol.ResumeRunResponse, iter.Seq[protocol.RunEvent], error)
	SubscribeRun(context.Context, protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error)
	CancelRun(context.Context, protocol.CancelRunRequest) (*protocol.CancelRunResponse, error)
	SteerRun(context.Context, protocol.SteerRunRequest) error
	GetRun(context.Context, protocol.GetRunRequest) (*protocol.RunRef, error)
	ListRuns(context.Context, protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error)
	ListInterrupts(context.Context, protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error)
	ListItems(context.Context, protocol.ListItemsRequest) (*protocol.ListItemsResponse, error)
	GetPlan(context.Context, protocol.GetPlanRequest) (*protocol.StateSnapshot, error)
	SubscribeRuntime(context.Context, protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error)
	ResolveWorkspace(context.Context, protocol.ResolveWorkspaceRequest) (*protocol.WorkspaceInfo, error)
	ListWorkspaces(context.Context) (*protocol.Page[protocol.WorkspaceSummary], error)
	ListWorkspaceFileChanges(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.WorkspaceFileChange], error)
	GetWorkspaceDiff(context.Context, protocol.GetDiffRequest) (*protocol.Diff, error)
	GetWorkspaceFileHead(context.Context, protocol.GetFileHeadRequest) (*protocol.FileHead, error)
	GrepWorkspace(context.Context, protocol.GrepRequest) (*protocol.GrepResult, error)
	ListWorkspaceFiles(context.Context, protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error)
	ReadWorkspaceFile(context.Context, protocol.ReadFileRequest) (*protocol.FileContent, error)
	ListDiscoveredSkills(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.Skill], error)
	ListManagedSkills(context.Context) (*protocol.Page[protocol.ManagedSkill], error)
	ArchiveSkill(context.Context, protocol.SkillNameRequest) error
	RestoreSkill(context.Context, protocol.SkillNameRequest) error
	ListSkillProposals(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error)
	ApproveSkillProposal(context.Context, protocol.SkillProposalRef) error
	RejectSkillProposal(context.Context, protocol.SkillProposalRef) error
	ListRecipes(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.Recipe], error)
	ListAgentDocs(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error)
	ListMCPServers(context.Context) (*protocol.Page[protocol.MCPServer], error)
	CreateMCPServer(context.Context, protocol.MCPServerCandidate) (*protocol.MCPServer, error)
	UpdateMCPServer(context.Context, protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error)
	DeleteMCPServer(context.Context, string) error
	TestMCPServer(context.Context, protocol.MCPServerCandidate) (*protocol.MCPTestResult, error)
	ListMCPTools(context.Context, protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error)
	ReconnectMCPServer(context.Context, string) error
	CreateMCPAuthorizationAttempt(context.Context, string) (*protocol.MCPAuthorizationAttempt, error)
	GetMCPAuthorizationAttempt(context.Context, string) (*protocol.MCPAuthorizationAttempt, error)
	ListHooks(context.Context, protocol.ListHooksRequest) (*protocol.HooksListResult, error)
	SetHookTrust(context.Context, protocol.SetHookTrustRequest) error
	GetApprovalMode(context.Context) (*protocol.ApprovalModeResult, error)
	SetApprovalMode(context.Context, protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error)
	ListApprovalRules(context.Context, protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error)
	ForgetApprovalRule(context.Context, protocol.ForgetApprovalRuleRequest) error
	ListSchedules(context.Context, protocol.PageQuery) (*protocol.Page[protocol.Schedule], error)
	CreateSchedule(context.Context, protocol.CreateScheduleRequest) (*protocol.Schedule, error)
	UpdateSchedule(context.Context, protocol.UpdateScheduleRequest) (*protocol.Schedule, error)
	DeleteSchedule(context.Context, protocol.DeleteScheduleRequest) error
	RunScheduleNow(context.Context, protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error)
	StartGoal(context.Context, protocol.StartGoalRequest) (*protocol.Goal, error)
	GetGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	StopGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	ResumeGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	CodebaseSearch(context.Context, protocol.CodebaseSearchRequest) (*protocol.CodebaseSearchResult, error)
	CodebaseStatus(context.Context, protocol.CodebaseStatusRequest) (*protocol.CodebaseStatus, error)
	CodebaseReindex(context.Context, protocol.CodebaseReindexRequest) (*protocol.CodebaseReindexResponse, error)
	ListProviders(context.Context) (*protocol.Page[protocol.Provider], error)
	UpdateProvider(context.Context, protocol.UpdateProviderRequest) (*protocol.Provider, error)
	TestProvider(context.Context, string) (*protocol.ProviderTestResult, error)
	ListModels(context.Context, protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error)
	GetUtilityRole(context.Context) (*protocol.UtilityRole, error)
	SetUtilityRole(context.Context, protocol.UtilityRole) (*protocol.UtilityRole, error)
	GetEmbeddingRole(context.Context) (*protocol.EmbeddingRole, error)
	SetEmbeddingRole(context.Context, protocol.EmbeddingRole) (*protocol.EmbeddingRole, error)
	ListTools(context.Context) (*protocol.Page[protocol.ToolSpec], error)
	InvokeTool(context.Context, protocol.InvokeToolRequest) (any, error)
	ListKnowledge(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error)
	GetKnowledge(context.Context, protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error)
	UpdateKnowledge(context.Context, protocol.UpdateKnowledgeRequest) (*protocol.KnowledgeEntry, error)
	ListAgentMemory(context.Context, protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error)
	ReviewAgentMemory(context.Context, protocol.AgentMemoryReviewRequest) error
	UpdateAgentMemory(context.Context, protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error)
	DeleteAgentMemory(context.Context, protocol.AgentMemoryItemRequest) error
	AddAgentMemory(context.Context, protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error)
	CreateFeedback(context.Context, protocol.FeedbackRequest) error
	SessionUsage(context.Context, string) (*protocol.Usage, error)
	UsageSummary(context.Context, protocol.UsageSummaryRequest) (*protocol.UsageSummary, error)
}
