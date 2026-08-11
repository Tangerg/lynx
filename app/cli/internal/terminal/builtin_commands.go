package terminal

import (
	"slices"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/modelconfig"
)

const (
	commandCategoryApplication = "Application"
	commandCategoryTranscript  = "Transcript"
	commandCategorySessions    = "Sessions"
	commandCategoryComposer    = "Composer"
	commandCategoryRuntime     = "Runtime"
	commandCategoryAutomation  = "Automation"
	commandCategoryContext     = "Context"
	commandCategoryConnections = "Connections"
	commandCategoryWorkspace   = "Workspace"
	commandCategoryExtensions  = "Extensions"
)

func builtinCommands() []localCommand {
	return slices.Concat(
		commandGroup(commandCategoryApplication,
			localCommand{Descriptor: CommandDescriptor{Name: "quit", Title: "leave lyra", Aliases: []string{"exit"}}, Run: func(a *app, _ string) error { a.Quit(); return nil }},
		),
		commandGroup(commandCategoryTranscript,
			localCommand{Descriptor: CommandDescriptor{Name: "help", Title: "show commands available in this session"}, Run: func(a *app, _ string) error { a.ShowHelp(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "shortcuts", Title: "show all keyboard shortcuts"}, Run: func(a *app, _ string) error { a.ShowShortcuts(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "clear", Title: "release the live transcript"}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { a.Clear(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "find", Title: "find text in the live transcript", Takes: true}, Run: func(a *app, query string) error { a.Find(query); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "next", Title: "step to the next search match"}, Run: func(a *app, _ string) error { a.NextMatch(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "previous", Title: "step to the previous search match", Aliases: []string{"prev"}}, Run: func(a *app, _ string) error { a.PreviousMatch(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "queue", Title: "manage follow-ups waiting behind the current run"}, Run: func(a *app, _ string) error { a.ShowQueue(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "details", Title: "expand or collapse tool output and diffs"}, Run: func(a *app, _ string) error { a.ToggleToolDetails(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "view", Title: "open the selected transcript entry in the full reader"}, Available: availableWithReadableSelection, Run: func(a *app, _ string) error { a.OpenReader(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "copy-last", Title: "copy the latest durable assistant response"}, Run: func(a *app, _ string) error { return a.copyLastAssistant() }},
			localCommand{Descriptor: CommandDescriptor{Name: "export", Title: "export a runtime-native session document", Takes: true}, Available: availableWithSessionTransfer, Run: func(a *app, argument string) error { return a.exportSession(argument) }},
		),
		commandGroup(commandCategorySessions,
			localCommand{Descriptor: CommandDescriptor{Name: "sessions", Title: "search and switch sessions", Aliases: []string{"resume"}}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { a.ShowSessions(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "timeline", Title: "browse runs in the current session"}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { a.ShowTimeline(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "new", Title: "start a new session"}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { a.NewSession(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "workspace", Title: "start a session in a recent or specified workspace"}, Available: availableWithoutActiveRun, Run: func(a *app, path string) error { return a.chooseWorkspace(path) }},
			localCommand{Descriptor: CommandDescriptor{Name: "rename", Title: "rename the current session", Takes: true}, Available: availableWithoutActiveRun, Run: func(a *app, title string) error { a.RenameSession(title); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "fork", Title: "fork the complete current session", Takes: true}, Available: availableWithoutActiveRun, Run: func(a *app, title string) error { a.ForkSession(title); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "rollback", Title: "rewind history, files, or both to a run boundary", Takes: true}, Available: availableForRollback, Run: func(a *app, argument string) error { return a.prepareSessionRollback(argument) }},
			localCommand{Descriptor: CommandDescriptor{Name: "import", Title: "import and open a runtime-native JSON session artifact", Takes: true}, Available: availableWithSessionTransfer, Run: func(a *app, path string) error { return a.prepareSessionImport(path) }},
		),
		commandGroup(commandCategoryComposer,
			localCommand{Descriptor: CommandDescriptor{Name: "attach", Title: "attach a local file to the next prompt", Takes: true}, Run: func(a *app, path string) error { return a.AttachFile(path) }},
			localCommand{Descriptor: CommandDescriptor{Name: "detach", Title: "remove an attachment by name, number, or all", Takes: true}, Run: func(a *app, value string) error { return a.DetachFile(value) }},
			localCommand{Descriptor: CommandDescriptor{Name: "attachments", Title: "show files attached to the next prompt", Aliases: []string{"files"}}, Run: func(a *app, _ string) error { a.ShowAttachments(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "stash", Title: "stash the current prompt and clear the composer"}, Available: availableWithDraft, Run: func(a *app, _ string) error { return a.stashPrompt() }},
			localCommand{Descriptor: CommandDescriptor{Name: "stashes", Title: "list saved prompt stashes"}, Run: func(a *app, _ string) error { a.showPromptStashes(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "stash-apply", Title: "apply a prompt stash by id or unique prefix", Takes: true}, Available: availableWithEmptyDraft, Run: func(a *app, id string) error { return a.applyPromptStash(id) }},
			localCommand{Descriptor: CommandDescriptor{Name: "stash-delete", Title: "delete a prompt stash by id or unique prefix", Takes: true}, Run: func(a *app, id string) error { return a.deletePromptStash(id) }},
			localCommand{Descriptor: CommandDescriptor{Name: "editor", Title: "edit the current prompt in the configured external editor"}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { return a.editPromptExternally() }},
		),
		commandGroup(commandCategoryRuntime,
			localCommand{Descriptor: CommandDescriptor{Name: "model", Title: "choose the model for new runs"}, Run: func(a *app, _ string) error { a.ChooseModel(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "usage", Title: "inspect session and runtime usage", Takes: true}, Available: availableWithUsage, Run: func(a *app, days string) error { return a.ShowUsage(days) }},
			localCommand{Descriptor: CommandDescriptor{Name: "roles", Title: "inspect utility and embedding model roles"}, Available: availableWithModelConfiguration, Run: func(a *app, _ string) error { a.ShowModelRoles(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "utility", Title: "set the utility model role", Takes: true}, Available: availableWithModelConfiguration, Run: func(a *app, target string) error { return a.SetModelRole(modelconfig.UtilityRole, target) }},
			localCommand{Descriptor: CommandDescriptor{Name: "embedding", Title: "set the embedding model role", Takes: true}, Available: availableWithModelConfiguration, Run: func(a *app, target string) error { return a.SetModelRole(modelconfig.EmbeddingRole, target) }},
			localCommand{Descriptor: CommandDescriptor{Name: "providers", Title: "inspect configured model providers"}, Available: availableWithModelConfiguration, Run: func(a *app, _ string) error { a.ShowProviders(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "provider-test", Title: "test a configured provider", Takes: true}, Available: availableWithModelConfiguration, Run: func(a *app, provider string) error { return a.TestConfiguredProvider(provider) }},
			localCommand{Descriptor: CommandDescriptor{Name: "provider-config", Title: "configure provider endpoint and credentials", Takes: true}, Available: availableWithModelConfiguration, Run: func(a *app, provider string) error { return a.ConfigureProvider(provider) }},
			localCommand{Descriptor: CommandDescriptor{Name: "approval", Title: "choose the runtime approval mode", Aliases: []string{"permissions", "permission"}}, Run: func(a *app, _ string) error { a.ChooseApprovalMode(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "status", Title: "show model, run limits, and runtime approval mode"}, Run: func(a *app, _ string) error { a.ShowRuntimeStatus(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "rules", Title: "show remembered approval rules"}, Run: func(a *app, _ string) error { a.ShowApprovalRules(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "steer", Title: "inject an instruction into the observed run segment", Takes: true}, Available: availableWithRunningSegment, Run: func(a *app, instruction string) error { return a.steerRun(instruction) }},
			localCommand{Descriptor: CommandDescriptor{Name: "goal", Title: "inspect the current autonomous session goal"}, Available: availableWithGoals, Run: func(a *app, _ string) error { a.ShowGoal(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "goal-start", Title: "start autonomous pursuit for this session", Takes: true}, Available: availableForGoalStart, Run: func(a *app, objective string) error { return a.StartGoal(objective) }},
			localCommand{Descriptor: CommandDescriptor{Name: "goal-stop", Title: "pause autonomous pursuit for this session"}, Available: availableWithGoals, Run: func(a *app, _ string) error { return a.StopGoal() }},
			localCommand{Descriptor: CommandDescriptor{Name: "goal-resume", Title: "resume autonomous pursuit for this session"}, Available: availableWithGoals, Run: func(a *app, _ string) error { return a.ResumeGoal() }},
		),
		commandGroup(commandCategoryAutomation,
			localCommand{Descriptor: CommandDescriptor{Name: "schedules", Title: "inspect scheduled headless runs"}, Available: availableWithSchedules, Run: func(a *app, _ string) error { a.ShowSchedules(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "schedule-create", Title: "create a scheduled headless run"}, Available: availableWithSchedules, Run: func(a *app, _ string) error { return a.OpenScheduleCreateForm() }},
			localCommand{Descriptor: CommandDescriptor{Name: "schedule-edit", Title: "edit a schedule by id, prefix, or title", Takes: true}, Available: availableWithSchedules, Run: func(a *app, identity string) error { return a.EditSchedule(identity) }},
			localCommand{Descriptor: CommandDescriptor{Name: "schedule-enable", Title: "enable a schedule", Takes: true}, Available: availableWithSchedules, Run: func(a *app, identity string) error { return a.SetScheduleEnabled(identity, true) }},
			localCommand{Descriptor: CommandDescriptor{Name: "schedule-disable", Title: "disable a schedule", Takes: true}, Available: availableWithSchedules, Run: func(a *app, identity string) error { return a.SetScheduleEnabled(identity, false) }},
			localCommand{Descriptor: CommandDescriptor{Name: "schedule-run", Title: "fire a schedule without advancing its cron cursor", Takes: true}, Available: availableWithSchedules, Run: func(a *app, identity string) error { return a.RunScheduleNow(identity) }},
			localCommand{Descriptor: CommandDescriptor{Name: "schedule-delete", Title: "delete a schedule", Takes: true}, Available: availableWithSchedules, Run: func(a *app, identity string) error { return a.PrepareDeleteSchedule(identity) }},
		),
		commandGroup(commandCategoryContext,
			localCommand{Descriptor: CommandDescriptor{Name: "memory", Title: "inspect governed agent memory by scope", Takes: true}, Available: availableWithAgentMemory, Run: func(a *app, scope string) error { return a.ShowAgentMemory(scope) }},
			localCommand{Descriptor: CommandDescriptor{Name: "memory-add", Title: "author a new active memory item", Takes: true}, Available: availableWithAgentMemory, Run: func(a *app, scope string) error { return a.AddAgentMemory(scope) }},
			localCommand{Descriptor: CommandDescriptor{Name: "memory-edit", Title: "edit memory by scope and id", Takes: true}, Available: availableWithAgentMemory, Run: func(a *app, identity string) error { return a.EditAgentMemory(identity) }},
			localCommand{Descriptor: CommandDescriptor{Name: "memory-pin", Title: "pin memory by scope and id", Takes: true}, Available: availableWithAgentMemory, Run: func(a *app, identity string) error { return a.SetAgentMemoryPinned(identity, true) }},
			localCommand{Descriptor: CommandDescriptor{Name: "memory-unpin", Title: "unpin memory by scope and id", Takes: true}, Available: availableWithAgentMemory, Run: func(a *app, identity string) error { return a.SetAgentMemoryPinned(identity, false) }},
			localCommand{Descriptor: CommandDescriptor{Name: "memory-approve", Title: "approve a pending memory proposal", Takes: true}, Available: availableWithAgentMemory, Run: func(a *app, identity string) error { return a.PrepareAgentMemoryReview(identity, true) }},
			localCommand{Descriptor: CommandDescriptor{Name: "memory-reject", Title: "reject a pending memory proposal", Takes: true}, Available: availableWithAgentMemory, Run: func(a *app, identity string) error { return a.PrepareAgentMemoryReview(identity, false) }},
			localCommand{Descriptor: CommandDescriptor{Name: "memory-delete", Title: "delete memory by scope and id", Takes: true}, Available: availableWithAgentMemory, Run: func(a *app, identity string) error { return a.PrepareDeleteAgentMemory(identity) }},
			localCommand{Descriptor: CommandDescriptor{Name: "knowledge", Title: "inspect the LYRA.md knowledge cascade"}, Available: availableWithKnowledge, Run: func(a *app, _ string) error { a.ShowKnowledge(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "knowledge-read", Title: "read one LYRA.md scope", Takes: true}, Available: availableWithKnowledge, Run: func(a *app, scope string) error { return a.ReadKnowledge(scope) }},
			localCommand{Descriptor: CommandDescriptor{Name: "knowledge-edit", Title: "edit one LYRA.md scope", Takes: true}, Available: availableWithKnowledge, Run: func(a *app, scope string) error { return a.EditKnowledge(scope) }},
			localCommand{Descriptor: CommandDescriptor{Name: "skills", Title: "inspect skills discoverable in this workspace"}, Available: availableWithSkills, Run: func(a *app, _ string) error { a.ShowDiscoveredSkills(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "skill-library", Title: "inspect active and archived managed skills"}, Available: availableWithSkills, Run: func(a *app, _ string) error { a.ShowManagedSkills(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "skill-proposals", Title: "review pending immutable Skill proposals"}, Available: availableWithSkills, Run: func(a *app, _ string) error { a.ShowSkillProposals(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "skill-archive", Title: "archive one managed Skill", Takes: true}, Available: availableWithSkills, Run: func(a *app, name string) error { return a.ArchiveSkill(name) }},
			localCommand{Descriptor: CommandDescriptor{Name: "skill-restore", Title: "restore one archived managed Skill", Takes: true}, Available: availableWithSkills, Run: func(a *app, name string) error { return a.RestoreSkill(name) }},
			localCommand{Descriptor: CommandDescriptor{Name: "skill-approve", Title: "approve an exact pending Skill proposal", Takes: true}, Available: availableWithSkills, Run: func(a *app, identity string) error { return a.PrepareSkillProposalDecision(identity, true) }},
			localCommand{Descriptor: CommandDescriptor{Name: "skill-reject", Title: "reject an exact pending Skill proposal", Takes: true}, Available: availableWithSkills, Run: func(a *app, identity string) error { return a.PrepareSkillProposalDecision(identity, false) }},
		),
		commandGroup(commandCategoryConnections,
			localCommand{Descriptor: CommandDescriptor{Name: "mcp", Title: "inspect configured MCP servers and live state"}, Available: availableWithMCP, Run: func(a *app, _ string) error { a.ShowMCPServers(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "mcp-tools", Title: "inspect MCP tools, optionally for one server", Takes: true}, Available: availableWithMCP, Run: func(a *app, server string) error { a.ShowMCPTools(server); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "mcp-create", Title: "configure a new MCP server"}, Available: availableWithMCP, Run: func(a *app, _ string) error { return a.OpenMCPCreateForm() }},
			localCommand{Descriptor: CommandDescriptor{Name: "mcp-edit", Title: "update an MCP server", Takes: true}, Available: availableWithMCP, Run: func(a *app, server string) error { return a.EditMCPServer(server) }},
			localCommand{Descriptor: CommandDescriptor{Name: "mcp-probe", Title: "test an unpersisted MCP candidate"}, Available: availableWithMCP, Run: func(a *app, _ string) error { return a.OpenMCPProbeForm() }},
			localCommand{Descriptor: CommandDescriptor{Name: "mcp-delete", Title: "delete an MCP server", Takes: true}, Available: availableWithMCP, Run: func(a *app, server string) error { return a.PrepareDeleteMCPServer(server) }},
			localCommand{Descriptor: CommandDescriptor{Name: "mcp-reconnect", Title: "request an asynchronous MCP reconnect", Takes: true}, Available: availableWithMCP, Run: func(a *app, server string) error { return a.ReconnectMCPServer(server) }},
			localCommand{Descriptor: CommandDescriptor{Name: "mcp-auth", Title: "start and observe MCP browser authorization", Takes: true}, Available: availableWithMCP, Run: func(a *app, server string) error { return a.AuthorizeMCPServer(server) }},
		),
		commandGroup(commandCategoryWorkspace,
			localCommand{Descriptor: CommandDescriptor{Name: "workspaces", Title: "inspect runtime-known workspaces"}, Available: availableWithWorkspaceService, Run: func(a *app, _ string) error { a.ShowWorkspaces(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "changes", Title: "inspect authoritative workspace changes"}, Available: availableWithWorkspaceService, Run: func(a *app, _ string) error { a.ShowWorkspaceChanges(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "diff", Title: "inspect the workspace diff, optionally for one path"}, Available: availableWithWorkspaceService, Run: func(a *app, path string) error { a.ShowWorkspaceDiff(path); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "preview", Title: "preview the first lines of a workspace file", Takes: true}, Available: availableWithWorkspaceService, Run: func(a *app, path string) error { a.PreviewWorkspaceFile(path); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "grep", Title: "search text across workspace files", Takes: true}, Available: availableWithWorkspaceService, Run: func(a *app, query string) error { a.SearchWorkspace(query); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "browse", Title: "browse a workspace directory"}, Available: availableWithWorkspaceService, Run: func(a *app, path string) error { a.BrowseWorkspace(path); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "read", Title: "read an authoritative workspace file", Takes: true}, Available: availableWithWorkspaceService, Run: func(a *app, path string) error { a.ReadWorkspaceFile(path); return nil }},
		),
		commandGroup(commandCategoryExtensions,
			localCommand{Descriptor: CommandDescriptor{Name: "plugins", Title: "show discovered plugins and lifecycle state"}, Run: func(a *app, _ string) error { a.ShowPlugins(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "reload", Title: "reload a plugin and its dependents", Takes: true}, Run: func(a *app, id string) error { a.ReloadPlugin(id); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "unload", Title: "unload a sideloaded plugin and its dependents", Takes: true}, Run: func(a *app, id string) error { a.UnloadPlugin(id); return nil }},
		),
	)
}

func availableWithWorkspaceService(a *app) CommandAvailability {
	if a.workspaces == nil {
		return CommandAvailability{Reason: "this runtime composition has no workspace service"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithSessionTransfer(a *app) CommandAvailability {
	if unavailable := availableWithoutActiveRun(a); !unavailable.Enabled {
		return unavailable
	}
	if a.transfers == nil {
		return CommandAvailability{Reason: "this runtime composition has no session transfer service"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithUsage(a *app) CommandAvailability {
	if a.usage == nil {
		return CommandAvailability{Reason: "this runtime composition has no usage service"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithModelConfiguration(a *app) CommandAvailability {
	if a.modelConfig == nil {
		return CommandAvailability{Reason: "this runtime composition has no model configuration service"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithGoals(a *app) CommandAvailability {
	if a.goals == nil {
		return CommandAvailability{Reason: "this runtime composition has no goal service"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithSkills(a *app) CommandAvailability {
	if a.skills == nil {
		return CommandAvailability{Reason: "this runtime composition has no skill service"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithMCP(a *app) CommandAvailability {
	if a.mcp == nil {
		return CommandAvailability{Reason: "this runtime composition has no MCP service"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithSchedules(a *app) CommandAvailability {
	if a.schedules == nil {
		return CommandAvailability{Reason: "this runtime composition has no schedule service"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithAgentMemory(a *app) CommandAvailability {
	if a.agentMemory == nil {
		return CommandAvailability{Reason: "this runtime composition has no agent memory service"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithKnowledge(a *app) CommandAvailability {
	if a.knowledge == nil {
		return CommandAvailability{Reason: "this runtime composition has no knowledge service"}
	}
	return CommandAvailability{Enabled: true}
}

func availableForGoalStart(a *app) CommandAvailability {
	if unavailable := availableWithoutActiveRun(a); !unavailable.Enabled {
		return unavailable
	}
	return availableWithGoals(a)
}

func availableForRollback(a *app) CommandAvailability {
	if unavailable := availableWithoutActiveRun(a); !unavailable.Enabled {
		return unavailable
	}
	_, present, err := a.currentDraft()
	if err != nil {
		return CommandAvailability{Reason: err.Error()}
	}
	if present {
		return CommandAvailability{Reason: "stash or detach the current draft before rolling back"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithRunningSegment(a *app) CommandAvailability {
	if a.conversation.Phase() != agent.ConversationRunning || a.conversation.RunID() == "" || a.conversation.SegmentID() == "" {
		return CommandAvailability{Reason: "no observed run segment is executing"}
	}
	return CommandAvailability{Enabled: true}
}

func commandGroup(category string, commands ...localCommand) []localCommand {
	for index := range commands {
		commands[index].Descriptor.Category = category
	}
	return commands
}

func availableWithoutActiveRun(a *app) CommandAvailability {
	if a.conversation.Busy() || a.following || a.pendingCancel != nil {
		return CommandAvailability{Reason: "an active run owns this session"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithReadableSelection(a *app) CommandAvailability {
	if _, readable := a.transcript.readerTargetForSelected(); !readable {
		return CommandAvailability{Reason: "select a readable transcript entry first"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithDraft(a *app) CommandAvailability {
	_, present, err := a.currentDraft()
	if err != nil {
		return CommandAvailability{Reason: err.Error()}
	}
	if !present {
		return CommandAvailability{Reason: "the composer is empty"}
	}
	return CommandAvailability{Enabled: true}
}

func availableWithEmptyDraft(a *app) CommandAvailability {
	_, present, err := a.currentDraft()
	if err != nil {
		return CommandAvailability{Reason: err.Error()}
	}
	if present {
		return CommandAvailability{Reason: "stash or clear the current draft first"}
	}
	return CommandAvailability{Enabled: true}
}

func (a *app) Clear() {
	if a.conversation.Busy() || a.following {
		a.status.doing = "the active run owns the transcript"
		return
	}
	a.conversation.ClearPresentation()
	a.transcript.Reset()
	a.activity.Reset()
	a.status.Reset(a.options)
	a.status.note("cleared")
	a.header.SetUsage(a.conversation.Usage())
}

func (a *app) Find(query string) {
	a.transcript.Find(query)
	a.message("searching for " + query)
}

func (a *app) NextMatch() {
	if !a.transcript.StepMatch(1) {
		a.message("no active search matches")
	}
}

func (a *app) PreviousMatch() {
	if !a.transcript.StepMatch(-1) {
		a.message("no active search matches")
	}
}

func (a *app) Quit() { a.loop.Quit() }

func (a *app) ShowHelp() { a.showCommandPalette() }

func (a *app) ShowShortcuts() { a.showShortcutDialog() }

func (a *app) AttachFile(path string) error { return a.addAttachment(path) }

func (a *app) DetachFile(value string) error { return a.removeAttachment(value) }

func (a *app) ShowAttachments() { a.showAttachments() }

func (a *app) ToggleToolDetails() {
	a.transcript.ToggleDetails()
	a.message(a.transcript.DetailsLabel())
}
