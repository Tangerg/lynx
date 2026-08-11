package terminal

import "slices"

const (
	commandCategoryApplication = "Application"
	commandCategoryTranscript  = "Transcript"
	commandCategorySessions    = "Sessions"
	commandCategoryComposer    = "Composer"
	commandCategoryRuntime     = "Runtime"
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
			localCommand{Descriptor: CommandDescriptor{Name: "export", Title: "export this session as markdown or json", Takes: true}, Run: func(a *app, argument string) error { return a.exportSession(argument) }},
		),
		commandGroup(commandCategorySessions,
			localCommand{Descriptor: CommandDescriptor{Name: "sessions", Title: "search and switch sessions", Aliases: []string{"resume"}}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { a.ShowSessions(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "timeline", Title: "browse runs in the current session"}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { a.ShowTimeline(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "new", Title: "start a new session"}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { a.NewSession(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "workspace", Title: "start a session in a recent or specified workspace"}, Available: availableWithoutActiveRun, Run: func(a *app, path string) error { return a.chooseWorkspace(path) }},
			localCommand{Descriptor: CommandDescriptor{Name: "rename", Title: "rename the current session", Takes: true}, Available: availableWithoutActiveRun, Run: func(a *app, title string) error { a.RenameSession(title); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "fork", Title: "fork the complete current session", Takes: true}, Available: availableWithoutActiveRun, Run: func(a *app, title string) error { a.ForkSession(title); return nil }},
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
			localCommand{Descriptor: CommandDescriptor{Name: "approval", Title: "choose the runtime approval mode", Aliases: []string{"permissions", "permission"}}, Run: func(a *app, _ string) error { a.ChooseApprovalMode(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "status", Title: "show model, run limits, and runtime approval mode"}, Run: func(a *app, _ string) error { a.ShowRuntimeStatus(); return nil }},
			localCommand{Descriptor: CommandDescriptor{Name: "rules", Title: "show remembered approval rules"}, Run: func(a *app, _ string) error { a.ShowApprovalRules(); return nil }},
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
