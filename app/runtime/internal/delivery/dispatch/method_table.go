package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// methodHandler decodes one request, calls the typed Runtime method,
// and encodes the result. Every business method shares this signature
// and routes through [methodTable] (CLAUDE.md: 查表法替代条件链).
type methodHandler = func(*Dispatcher, context.Context, *transport.Request) HandleResult

// methodTable maps each JSON-RPC method name to its handler. Handlers
// live in domain-grouped files; adding a method = one handler + one
// row. Notifications route through [Dispatcher.handleNotification].
var methodTable = map[string]methodHandler{
	// Lifecycle.
	MethodDiscover: (*Dispatcher).handleDiscover,

	// Sessions.
	MethodSessionsList:     (*Dispatcher).handleSessionsList,
	MethodSessionsGet:      (*Dispatcher).handleSessionsGet,
	MethodSessionsCreate:   (*Dispatcher).handleSessionsCreate,
	MethodSessionsUpdate:   (*Dispatcher).handleSessionsUpdate,
	MethodSessionsDelete:   (*Dispatcher).handleSessionsDelete,
	MethodSessionsFork:     (*Dispatcher).handleSessionsFork,
	MethodSessionsRollback: (*Dispatcher).handleSessionsRollback,
	MethodSessionsExport:   (*Dispatcher).handleSessionsExport,
	MethodSessionsImport:   (*Dispatcher).handleSessionsImport,

	// Runs.
	MethodRunsStart:              (*Dispatcher).handleRunsStart,
	MethodRunsResume:             (*Dispatcher).handleRunsResume,
	MethodRunsSubscribe:          (*Dispatcher).handleRunsSubscribe,
	MethodRunsCancel:             (*Dispatcher).handleRunsCancel,
	MethodRunsSteer:              (*Dispatcher).handleRunsSteer,
	MethodRunsList:               (*Dispatcher).handleRunsList,
	MethodRunsListOpenInterrupts: (*Dispatcher).handleRunsListOpenInterrupts,

	// Items.
	MethodItemsList: (*Dispatcher).handleItemsList,

	// Workspace.
	MethodWorkspaceListFileChanges: (*Dispatcher).handleWorkspaceListFileChanges,
	MethodWorkspaceGetDiff:         (*Dispatcher).handleWorkspaceGetDiff,
	MethodWorkspaceGetFileHead:     (*Dispatcher).handleWorkspaceGetFileHead,
	MethodWorkspaceGrep:            (*Dispatcher).handleWorkspaceGrep,
	MethodWorkspaceListFiles:       (*Dispatcher).handleWorkspaceListFiles,
	MethodWorkspaceReadFile:        (*Dispatcher).handleWorkspaceReadFile,
	MethodWorkspaceListProjects:    (*Dispatcher).handleWorkspaceListProjects,
	MethodWorkspaceSubscribe:       (*Dispatcher).handleWorkspaceSubscribe,

	// Discovery and integrations.
	MethodSkillsDiscoveredList: (*Dispatcher).handleSkillsDiscoveredList,
	MethodSkillsLibraryList:    (*Dispatcher).handleSkillsLibraryList,
	MethodSkillsLibraryArchive: (*Dispatcher).handleSkillsLibraryArchive,
	MethodSkillsLibraryRestore: (*Dispatcher).handleSkillsLibraryRestore,
	MethodRecipesList:          (*Dispatcher).handleRecipesList,
	MethodAgentDocsList:        (*Dispatcher).handleAgentDocsList,
	MethodMCPServersList:       (*Dispatcher).handleMCPServersList,
	MethodMCPServersReconnect:  (*Dispatcher).handleMCPServersReconnect,
	MethodMCPServersAuthorize:  (*Dispatcher).handleMCPServersAuthorize,
	MethodMCPToolsList:         (*Dispatcher).handleMCPToolsList,
	MethodMCPConfigsList:       (*Dispatcher).handleMCPConfigsList,
	MethodMCPConfigsConfigure:  (*Dispatcher).handleMCPConfigsConfigure,
	MethodMCPConfigsRemove:     (*Dispatcher).handleMCPConfigsRemove,
	MethodMCPConfigsSetEnabled: (*Dispatcher).handleMCPConfigsSetEnabled,
	MethodMCPConfigsTest:       (*Dispatcher).handleMCPConfigsTest,
	MethodHooksList:            (*Dispatcher).handleHooksList,
	MethodHooksSetTrust:        (*Dispatcher).handleHooksSetTrust,

	// Approval.
	MethodApprovalGetMode:    (*Dispatcher).handleApprovalGetMode,
	MethodApprovalSetMode:    (*Dispatcher).handleApprovalSetMode,
	MethodApprovalListRules:  (*Dispatcher).handleApprovalListRules,
	MethodApprovalForgetRule: (*Dispatcher).handleApprovalForgetRule,

	// Schedules.
	MethodSchedulesList:   (*Dispatcher).handleSchedulesList,
	MethodSchedulesCreate: (*Dispatcher).handleSchedulesCreate,
	MethodSchedulesUpdate: (*Dispatcher).handleSchedulesUpdate,
	MethodSchedulesDelete: (*Dispatcher).handleSchedulesDelete,
	MethodSchedulesRunNow: (*Dispatcher).handleSchedulesRunNow,

	// Goals.
	MethodGoalsStart:  (*Dispatcher).handleGoalsStart,
	MethodGoalsGet:    (*Dispatcher).handleGoalsGet,
	MethodGoalsStop:   (*Dispatcher).handleGoalsStop,
	MethodGoalsResume: (*Dispatcher).handleGoalsResume,

	// Codebase.
	MethodCodebaseSearch:  (*Dispatcher).handleCodebaseSearch,
	MethodCodebaseStatus:  (*Dispatcher).handleCodebaseStatus,
	MethodCodebaseReindex: (*Dispatcher).handleCodebaseReindex,

	// Providers / Models / Tools.
	MethodProvidersList:          (*Dispatcher).handleProvidersList,
	MethodProvidersConfigure:     (*Dispatcher).handleProvidersConfigure,
	MethodProvidersTest:          (*Dispatcher).handleProvidersTest,
	MethodModelsList:             (*Dispatcher).handleModelsList,
	MethodModelsGetUtilityRole:   (*Dispatcher).handleModelsGetUtilityRole,
	MethodModelsSetUtilityRole:   (*Dispatcher).handleModelsSetUtilityRole,
	MethodModelsGetEmbeddingRole: (*Dispatcher).handleModelsGetEmbeddingRole,
	MethodModelsSetEmbeddingRole: (*Dispatcher).handleModelsSetEmbeddingRole,
	MethodToolsList:              (*Dispatcher).handleToolsList,
	MethodToolsInvoke:            (*Dispatcher).handleToolsInvoke,

	// Usage reporting.
	MethodUsageSession: (*Dispatcher).handleUsageSession,
	MethodUsageSummary: (*Dispatcher).handleUsageSummary,

	// Memory.
	MethodMemoryList:   (*Dispatcher).handleMemoryList,
	MethodMemoryGet:    (*Dispatcher).handleMemoryGet,
	MethodMemoryUpdate: (*Dispatcher).handleMemoryUpdate,

	// Agent memory (HITL review).
	MethodAgentMemoryList:   (*Dispatcher).handleAgentMemoryList,
	MethodAgentMemoryReview: (*Dispatcher).handleAgentMemoryReview,
	MethodAgentMemoryUpdate: (*Dispatcher).handleAgentMemoryUpdate,
	MethodAgentMemoryDelete: (*Dispatcher).handleAgentMemoryDelete,
	MethodAgentMemoryAdd:    (*Dispatcher).handleAgentMemoryAdd,

	// Feedback.
	MethodFeedbackCreate: (*Dispatcher).handleFeedbackCreate,
}

// dispatchRequest routes the request to its handler via [methodTable].
// Unknown methods get method_not_found.
func (d *Dispatcher) dispatchRequest(ctx context.Context, msg *transport.Request) HandleResult {
	handle, ok := methodTable[msg.Method]
	if !ok {
		return responseError(msg.ID, methodNotFound(msg.Method))
	}
	return handle(d, ctx, msg)
}
