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
	covered := runtimeAPIConsumptionByMethod()
	validModes := []runtimeConsumptionMode{
		consumedByLifecycle, consumedByCoreFlow, consumedByCommand, consumedBySideChannel,
	}
	for method, consumption := range covered {
		if consumption.Area == "" || consumption.Entry == "" {
			t.Fatalf("runtime method %s has no concrete product consumption path: %+v", method, consumption)
		}
		if !slices.Contains(validModes, consumption.Mode) {
			t.Fatalf("runtime method %s has invalid consumption mode %q", method, consumption.Mode)
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
	if preference := meta.ClientCapabilities.Features[protocol.FeatureSubagents]; !preference.Enabled {
		t.Fatal("request metadata does not negotiate the supported subagent stream profile")
	}
	wantInterrupts := supportedInterruptTypes()
	if !slices.Equal(meta.ClientCapabilities.InterruptTypes, wantInterrupts) {
		t.Fatalf("negotiated interrupts = %v, projection supports %v", meta.ClientCapabilities.InterruptTypes, wantInterrupts)
	}
	if len(meta.ClientCapabilities.ExcludedEphemeralEvents) != 0 {
		t.Fatalf("client unexpectedly suppresses runtime events: %v", meta.ClientCapabilities.ExcludedEphemeralEvents)
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

type runtimeConsumptionMode string

const (
	consumedByLifecycle   runtimeConsumptionMode = "lifecycle"
	consumedByCoreFlow    runtimeConsumptionMode = "core-flow"
	consumedByCommand     runtimeConsumptionMode = "command"
	consumedBySideChannel runtimeConsumptionMode = "side-channel"
)

type runtimeAPIConsumption struct {
	Area  string
	Mode  runtimeConsumptionMode
	Entry string
}

func runtimeAPIConsumptionByMethod() map[string]runtimeAPIConsumption {
	lifecycle := func(area, entry string) runtimeAPIConsumption {
		return runtimeAPIConsumption{Area: area, Mode: consumedByLifecycle, Entry: entry}
	}
	flow := func(area, entry string) runtimeAPIConsumption {
		return runtimeAPIConsumption{Area: area, Mode: consumedByCoreFlow, Entry: entry}
	}
	command := func(area, entry string) runtimeAPIConsumption {
		return runtimeAPIConsumption{Area: area, Mode: consumedByCommand, Entry: entry}
	}
	sideChannel := func(area, entry string) runtimeAPIConsumption {
		return runtimeAPIConsumption{Area: area, Mode: consumedBySideChannel, Entry: entry}
	}
	return map[string]runtimeAPIConsumption{
		"Discover": lifecycle("lifecycle", "embedded runtime startup negotiation"),
		"Close":    lifecycle("lifecycle", "process-owned backend shutdown"),

		"CreateSession":   flow("sessions", "interactive and one-shot session opening"),
		"DeleteSession":   command("sessions", "sessions delete"),
		"ExportSession":   command("sessions", "TUI /export"),
		"ForkSession":     command("sessions", "sessions fork and TUI /fork"),
		"GetSession":      flow("sessions", "cold restore, recovery, and sessions show"),
		"ImportSession":   command("sessions", "TUI /import"),
		"ListSessions":    command("sessions", "sessions ls, completion, and session picker"),
		"RollbackSession": command("sessions", "TUI /rollback"),
		"UpdateSession":   command("sessions", "sessions update/rename and TUI metadata actions"),

		"CancelRun":    command("runs", "run cleanup, TUI stop, and runs cancel"),
		"GetRun":       command("runs", "runs show"),
		"ListRuns":     command("runs", "runs ls/completion and cold session projection"),
		"ResumeRun":    flow("runs", "HITL continuation"),
		"StartRun":     flow("runs", "interactive and one-shot execution"),
		"SteerRun":     command("runs", "TUI /steer"),
		"SubscribeRun": flow("runs", "stream recovery and reattachment"),

		"GetPlan":        flow("run resources", "authoritative cold session projection"),
		"ListInterrupts": flow("run resources", "HITL cold restore"),
		"ListItems":      flow("run resources", "authoritative transcript restore"),

		"SubscribeRuntime": sideChannel("runtime events", "TUI invalidation and workspace file-change monitor"),

		"GetWorkspaceDiff":              command("workspaces", "TUI /diff"),
		"GetWorkspaceFileHead":          command("workspaces", "workspace change inspector"),
		"ListWorkspaceFileChanges":      sideChannel("workspaces", "files.changed authoritative reconciliation"),
		"ListWorkspaceFiles":            command("workspaces", "workspace browser"),
		"ListWorkspaces":                command("workspaces", "TUI /workspace picker"),
		"ReadWorkspaceFile":             command("workspaces", "workspace file reader"),
		"ResolveWorkspace":              flow("workspaces", "session workspace identity resolution"),
		"SearchWorkspaceFiles":          command("workspaces", "workspace file search"),
		"ListModels":                    command("models", "model picker and runtime status"),
		"GetSessionUsage":               command("usage", "TUI /usage session view"),
		"GetUsageSummary":               command("usage", "TUI /usage runtime summary"),
		"GetEmbeddingRole":              command("model roles", "TUI /roles"),
		"GetUtilityRole":                command("model roles", "TUI /roles"),
		"SetEmbeddingRole":              command("model roles", "TUI /embedding"),
		"SetUtilityRole":                command("model roles", "TUI /utility"),
		"ListProviders":                 command("providers", "TUI /providers and provider forms"),
		"TestProvider":                  command("providers", "TUI /provider-test"),
		"UpdateProvider":                command("providers", "TUI /provider-config"),
		"ForgetApprovalRule":            command("approvals", "approvals delete and TUI rule deletion"),
		"GetApprovalMode":               command("approvals", "TUI approval/status surfaces"),
		"ListApprovalRules":             command("approvals", "approvals ls/completion and TUI /rules"),
		"SetApprovalMode":               command("approvals", "TUI /approval"),
		"GetGoal":                       command("goals", "TUI /goal"),
		"ResumeGoal":                    command("goals", "TUI /goal-resume"),
		"StartGoal":                     command("goals", "TUI /goal-start"),
		"StopGoal":                      command("goals", "TUI /goal-stop"),
		"ApproveSkillProposal":          command("skills", "TUI /skill-approve"),
		"ArchiveSkill":                  command("skills", "TUI /skill-archive"),
		"ListDiscoveredSkills":          command("skills", "TUI /skills"),
		"ListManagedSkills":             command("skills", "TUI /skill-library"),
		"ListSkillProposals":            command("skills", "TUI /skill-proposals"),
		"RejectSkillProposal":           command("skills", "TUI /skill-reject"),
		"RestoreSkill":                  command("skills", "TUI /skill-restore"),
		"CreateMCPAuthorizationAttempt": command("MCP", "TUI /mcp-connect authorization flow"),
		"CreateMCPServer":               command("MCP", "TUI /mcp-add"),
		"DeleteMCPServer":               command("MCP", "TUI /mcp-delete"),
		"GetMCPAuthorizationAttempt":    command("MCP", "TUI MCP authorization polling"),
		"ListMCPServers":                command("MCP", "TUI /mcp and connection forms"),
		"ListMCPTools":                  command("MCP", "TUI /mcp-tools"),
		"ReconnectMCPServer":            command("MCP", "TUI /mcp-reconnect"),
		"TestMCPServer":                 command("MCP", "TUI /mcp-test"),
		"UpdateMCPServer":               command("MCP", "TUI /mcp-edit"),
		"CreateSchedule":                command("schedules", "TUI /schedule-create"),
		"DeleteSchedule":                command("schedules", "TUI /schedule-delete"),
		"ListSchedules":                 command("schedules", "TUI /schedules and schedule forms"),
		"RunScheduleNow":                command("schedules", "TUI /schedule-run"),
		"UpdateSchedule":                command("schedules", "TUI schedule edit/enable/disable"),
		"AddAgentMemory":                command("agent memory", "TUI /memory-add"),
		"DeleteAgentMemory":             command("agent memory", "TUI /memory-delete"),
		"ListAgentMemory":               command("agent memory", "TUI /memory and memory forms"),
		"ReviewAgentMemory":             command("agent memory", "TUI /memory-approve and /memory-reject"),
		"UpdateAgentMemory":             command("agent memory", "TUI memory edit/pin/unpin"),
		"GetKnowledge":                  command("knowledge", "TUI /knowledge-read and editor"),
		"ListKnowledge":                 command("knowledge", "TUI /knowledge"),
		"UpdateKnowledge":               command("knowledge", "TUI /knowledge-edit"),
		"InvokeTool":                    command("diagnostic tools", "TUI /tool-invoke"),
		"ListTools":                     command("diagnostic tools", "TUI /tools"),
		"GetCodebaseStatus":             command("codebase", "TUI /codebase"),
		"ReindexCodebase":               command("codebase", "TUI /codebase-reindex"),
		"SearchCodebase":                command("codebase", "TUI /codebase-search"),
		"ListAgentDocs":                 command("authoring context", "TUI /agent-docs"),
		"ListRecipes":                   command("authoring context", "TUI /recipes and /recipe"),
		"ListHooks":                     command("hooks", "TUI /hooks"),
		"SetHookTrust":                  command("hooks", "TUI /hooks-trust and /hooks-revoke"),
		"CreateFeedback":                command("feedback", "TUI /feedback"),
	}
}
