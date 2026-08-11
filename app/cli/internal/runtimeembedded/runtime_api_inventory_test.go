package runtimeembedded

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
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

func TestRuntimeTopicInventoryHasNoUnreviewedTopics(t *testing.T) {
	t.Parallel()
	protocolTopics := protocol.RuntimeTopics()
	clientTopics := changefeed.Topics()
	got := make([]changefeed.Topic, 0, len(protocolTopics))
	for _, topic := range protocolTopics {
		got = append(got, changefeed.Topic(topic))
	}
	if !slices.Equal(got, clientTopics) {
		t.Fatalf("runtime topic inventory drifted: protocol=%v client=%v", got, clientTopics)
	}
}

func TestNegotiatedRunCapabilitiesMatchProjectionBoundary(t *testing.T) {
	t.Parallel()
	meta := requestMeta("test")
	if meta.ClientCapabilities == nil {
		t.Fatal("request metadata omitted client capabilities")
	}
	wantInterrupts := supportedInterruptTypes()
	if !slices.Equal(meta.ClientCapabilities.InterruptTypes, wantInterrupts) {
		t.Fatalf("negotiated interrupts = %v, projection supports %v", meta.ClientCapabilities.InterruptTypes, wantInterrupts)
	}
	wantExcluded := excludedEphemeralRunEvents()
	if !slices.Equal(meta.ClientCapabilities.ExcludedEphemeralEvents, wantExcluded) {
		t.Fatalf("excluded events = %v, policy excludes %v", meta.ClientCapabilities.ExcludedEphemeralEvents, wantExcluded)
	}
	for _, eventType := range requiredRunEventTypes() {
		if !slices.Contains(recognizedRunEventTypes(), eventType) {
			t.Fatalf("required run event %q has no projection policy", eventType)
		}
	}
}

func TestDiscoveryRejectsUnprojectedStreamAndChangeCapabilities(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*protocol.DiscoverResponse)
	}{
		{
			name: "run event",
			mutate: func(discovery *protocol.DiscoverResponse) {
				discovery.Capabilities.RunEvents = append(discovery.Capabilities.RunEvents, "vendor.authoritative")
			},
		},
		{
			name: "runtime topic",
			mutate: func(discovery *protocol.DiscoverResponse) {
				discovery.Capabilities.RuntimeTopics = append(discovery.Capabilities.RuntimeTopics, "indexes.changed")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			discovery := compatibleDiscovery()
			test.mutate(discovery)
			if err := validateDiscovery(discovery); !errors.Is(err, agent.ErrIncompatibleRuntime) {
				t.Fatalf("validateDiscovery = %v, want ErrIncompatibleRuntime", err)
			}
		})
	}
}

func compatibleDiscovery() *protocol.DiscoverResponse {
	return &protocol.DiscoverResponse{
		Protocol: protocol.SupportedProtocolRange(),
		Capabilities: protocol.ServerCapabilities{
			RunEvents:        recognizedRunEventTypes(),
			RuntimeTopics:    protocol.RuntimeTopics(),
			StreamingMethods: []string{"runs.start", "runs.resume", "runs.subscribe"},
			StateSnapshots: []protocol.StateSnapshotCapability{{
				Key: protocol.StatePlan, RecoveryMethod: "plan.get",
				Scope: protocol.StateScopeSession, Writer: protocol.StateWriterRootRun,
			}},
			Limits: protocol.RuntimeLimits{RunReplay: protocol.RunReplayLimits{
				Scope: protocol.ReplayScopeRuntimeInstanceRootSegment,
			}},
		},
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
