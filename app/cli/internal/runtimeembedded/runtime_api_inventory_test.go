package runtimeembedded

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
)

func TestRuntimeAPIInventoryHasNoUnreviewedMethods(t *testing.T) {
	t.Parallel()
	covered := make(map[string]string)
	for area, methods := range runtimeAPIMethodsByArea() {
		for _, method := range methods {
			if previous, duplicate := covered[method]; duplicate {
				t.Fatalf("runtime method %s is assigned to both %s and %s", method, previous, area)
			}
			covered[method] = area
		}
	}

	runtimeType := reflect.TypeFor[*embedded.Runtime]()
	exported := make(map[string]struct{}, runtimeType.NumMethod())
	for index := range runtimeType.NumMethod() {
		exported[runtimeType.Method(index).Name] = struct{}{}
	}

	var missing, unreviewed []string
	for method := range covered {
		if _, exists := exported[method]; !exists {
			missing = append(missing, method)
		}
	}
	for method := range exported {
		if _, exists := covered[method]; !exists {
			unreviewed = append(unreviewed, method)
		}
	}
	slices.Sort(missing)
	slices.Sort(unreviewed)
	if len(missing) > 0 || len(unreviewed) > 0 {
		t.Fatalf("runtime API inventory drifted: removed=%v unreviewed=%v", missing, unreviewed)
	}
}

func runtimeAPIMethodsByArea() map[string][]string {
	return map[string][]string{
		"lifecycle": {
			"Discover", "Close",
		},
		"sessions": {
			"CreateSession", "DeleteSession", "ExportSession", "ForkSession", "GetSession",
			"ImportSession", "ListSessions", "RollbackSession", "UpdateSession",
		},
		"runs": {
			"CancelRun", "GetRun", "ListRuns", "ResumeRun", "StartRun", "SteerRun", "SubscribeRun",
		},
		"run resources": {
			"GetPlan", "ListInterrupts", "ListItems",
		},
		"runtime events": {
			"SubscribeRuntime",
		},
		"workspaces": {
			"GetWorkspaceDiff", "GetWorkspaceFileHead", "ListWorkspaceFileChanges", "ListWorkspaceFiles",
			"ListWorkspaces", "ReadWorkspaceFile", "ResolveWorkspace", "SearchWorkspaceFiles",
		},
		"models": {
			"ListModels",
		},
		"usage": {
			"GetSessionUsage", "GetUsageSummary",
		},
		"model roles": {
			"GetEmbeddingRole", "GetUtilityRole", "SetEmbeddingRole", "SetUtilityRole",
		},
		"providers": {
			"ListProviders", "TestProvider", "UpdateProvider",
		},
		"approvals": {
			"ForgetApprovalRule", "GetApprovalMode", "ListApprovalRules", "SetApprovalMode",
		},
		"goals": {
			"GetGoal", "ResumeGoal", "StartGoal", "StopGoal",
		},
		"skills": {
			"ApproveSkillProposal", "ArchiveSkill", "ListDiscoveredSkills", "ListManagedSkills",
			"ListSkillProposals", "RejectSkillProposal", "RestoreSkill",
		},
		"MCP": {
			"CreateMCPAuthorizationAttempt", "CreateMCPServer", "DeleteMCPServer", "GetMCPAuthorizationAttempt",
			"ListMCPServers", "ListMCPTools", "ReconnectMCPServer", "TestMCPServer", "UpdateMCPServer",
		},
		"schedules": {
			"CreateSchedule", "DeleteSchedule", "ListSchedules", "RunScheduleNow", "UpdateSchedule",
		},
		"agent memory": {
			"AddAgentMemory", "DeleteAgentMemory", "ListAgentMemory", "ReviewAgentMemory", "UpdateAgentMemory",
		},
		"knowledge": {
			"GetKnowledge", "ListKnowledge", "UpdateKnowledge",
		},
		"diagnostic tools": {
			"InvokeTool", "ListTools",
		},
		"codebase": {
			"GetCodebaseStatus", "ReindexCodebase", "SearchCodebase",
		},
		"authoring context": {
			"ListAgentDocs", "ListRecipes",
		},
		"hooks": {
			"ListHooks", "SetHookTrust",
		},
		"feedback": {
			"CreateFeedback",
		},
	}
}
