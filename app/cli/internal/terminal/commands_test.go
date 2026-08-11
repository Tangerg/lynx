package terminal

import (
	"slices"
	"strings"
	"testing"
)

func TestCommandDescriptorValidatesItsIdentityNamespace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		descriptor CommandDescriptor
		want       string
	}{
		{name: "valid", descriptor: CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"look"}}},
		{name: "missing name", descriptor: CommandDescriptor{Title: "inspect workspace"}, want: "has no name"},
		{name: "invalid name", descriptor: CommandDescriptor{Name: "in spect", Title: "inspect workspace"}, want: "invalid name"},
		{name: "missing title", descriptor: CommandDescriptor{Name: "inspect"}, want: "has no title"},
		{name: "invalid arguments", descriptor: CommandDescriptor{Name: "inspect", Title: "inspect workspace", Arguments: ArgumentMode(99)}, want: "argument mode"},
		{name: "invalid alias", descriptor: CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"bad alias"}}, want: "invalid alias"},
		{name: "duplicate alias", descriptor: CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"look", "look"}}, want: "repeats name or alias"},
		{name: "alias repeats name", descriptor: CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"inspect"}}, want: "repeats name or alias"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.descriptor.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestArgumentModeValidatesInvocations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		mode     ArgumentMode
		argument string
		want     string
	}{
		{name: "none empty", mode: NoArguments},
		{name: "none populated", mode: NoArguments, argument: "surprise", want: "does not accept"},
		{name: "optional empty", mode: OptionalArguments},
		{name: "optional populated", mode: OptionalArguments, argument: "value"},
		{name: "required empty", mode: RequiredArguments, want: "needs an argument"},
		{name: "required populated", mode: RequiredArguments, argument: "value"},
		{name: "invalid", mode: ArgumentMode(99), want: "invalid argument contract"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.mode.ValidateInvocation("inspect", test.argument)
			if test.want == "" && err != nil {
				t.Fatalf("ValidateInvocation() error = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("ValidateInvocation() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestCommandsWithUsefulDefaultsDeclareOptionalArguments(t *testing.T) {
	t.Parallel()
	want := map[string]bool{
		"workspace": true, "fork": true, "usage": true, "memory": true, "memory-add": true,
		"mcp-tools": true, "diff": true, "browse": true,
	}
	for _, command := range builtinCommands() {
		if !want[command.Descriptor.Name] {
			continue
		}
		delete(want, command.Descriptor.Name)
		if command.Descriptor.Arguments != OptionalArguments {
			t.Errorf("/%s arguments = %v, want optional", command.Descriptor.Name, command.Descriptor.Arguments)
		}
	}
	for name := range want {
		t.Errorf("defaultable command /%s is not registered", name)
	}
}

func TestCommandCatalogRejectsNameAndAliasConflicts(t *testing.T) {
	t.Parallel()
	catalog := newCommandCatalog()
	first := CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"look"}}
	if err := catalog.add("first", first, func(string) {}, nil); err != nil {
		t.Fatal(err)
	}
	conflicting := CommandDescriptor{Name: "look", Title: "conflict with an alias"}
	if err := catalog.add("second", conflicting, func(string) {}, nil); err == nil {
		t.Fatal("alias conflict was accepted")
	}
}

func TestSplitCommandArgumentPreservesRemainderAcrossWhitespace(t *testing.T) {
	t.Parallel()
	identity, remainder, ok := splitCommandArgument("  inspect\t{\"depth\": 2}  ")
	if !ok || identity != "inspect" || remainder != `{"depth": 2}` {
		t.Fatalf("split = (%q, %q, %t)", identity, remainder, ok)
	}
	if _, _, ok := splitCommandArgument(" \n\t "); ok {
		t.Fatal("empty argument was accepted")
	}
	if remainder, ok := trimCommandIdentity("review code\tcarefully", "review code"); !ok || remainder != "carefully" {
		t.Fatalf("trim identity = (%q, %t)", remainder, ok)
	}
	if _, ok := trimCommandIdentity("reviewer", "review"); ok {
		t.Fatal("partial token matched a complete identity")
	}
}

func TestBuiltinCommandsOwnTheirCategoryAndAvailabilityPolicy(t *testing.T) {
	t.Parallel()
	wantCategories := map[string][]string{
		commandCategoryApplication: {"quit"},
		commandCategoryTranscript:  {"help", "shortcuts", "clear", "find", "next", "previous", "queue", "details", "view", "copy-last", "export", "feedback"},
		commandCategorySessions:    {"sessions", "timeline", "new", "workspace", "rename", "fork", "rollback", "import"},
		commandCategoryComposer:    {"attach", "detach", "attachments", "stash", "stashes", "stash-apply", "stash-delete", "editor"},
		commandCategoryRuntime:     {"tools", "tool-invoke", "model", "usage", "roles", "utility", "embedding", "providers", "provider-test", "provider-config", "approval", "status", "rules", "steer", "goal", "goal-start", "goal-stop", "goal-resume", "hooks", "hooks-trust", "hooks-revoke"},
		commandCategoryAutomation:  {"schedules", "schedule-create", "schedule-edit", "schedule-enable", "schedule-disable", "schedule-run", "schedule-delete"},
		commandCategoryContext:     {"agent-docs", "recipes", "recipe", "memory", "memory-add", "memory-edit", "memory-pin", "memory-unpin", "memory-approve", "memory-reject", "memory-delete", "knowledge", "knowledge-read", "knowledge-edit", "skills", "skill-library", "skill-proposals", "skill-archive", "skill-restore", "skill-approve", "skill-reject"},
		commandCategoryConnections: {"mcp", "mcp-tools", "mcp-create", "mcp-edit", "mcp-probe", "mcp-delete", "mcp-reconnect", "mcp-auth"},
		commandCategoryWorkspace:   {"codebase", "codebase-search", "codebase-reindex", "workspaces", "changes", "diff", "preview", "grep", "browse", "read"},
		commandCategoryExtensions:  {"plugins", "reload", "unload"},
	}
	wantGuard := map[string]bool{
		"clear": true, "view": true, "export": true,
		"sessions": true, "timeline": true, "new": true, "workspace": true, "rename": true, "fork": true, "rollback": true, "import": true,
		"stash": true, "stash-apply": true, "editor": true,
		"workspaces": true, "changes": true, "diff": true, "preview": true, "grep": true, "browse": true, "read": true,
		"usage": true, "roles": true, "utility": true, "embedding": true, "providers": true, "provider-test": true, "provider-config": true,
		"steer": true, "goal": true, "goal-start": true, "goal-stop": true, "goal-resume": true,
		"skills": true, "skill-library": true, "skill-proposals": true, "skill-archive": true, "skill-restore": true, "skill-approve": true, "skill-reject": true,
		"memory": true, "memory-add": true, "memory-edit": true, "memory-pin": true, "memory-unpin": true, "memory-approve": true, "memory-reject": true, "memory-delete": true,
		"knowledge": true, "knowledge-read": true, "knowledge-edit": true,
		"mcp": true, "mcp-tools": true, "mcp-create": true, "mcp-edit": true, "mcp-probe": true, "mcp-delete": true, "mcp-reconnect": true, "mcp-auth": true,
		"schedules": true, "schedule-create": true, "schedule-edit": true, "schedule-enable": true, "schedule-disable": true, "schedule-run": true, "schedule-delete": true,
		"tools": true, "tool-invoke": true,
		"codebase": true, "codebase-search": true, "codebase-reindex": true,
		"agent-docs": true, "recipes": true, "recipe": true,
		"hooks": true, "hooks-trust": true, "hooks-revoke": true,
		"feedback": true,
	}

	seen := make(map[string]struct{})
	gotCategories := make(map[string][]string)
	for _, command := range builtinCommands() {
		if err := command.validate(); err != nil {
			t.Fatalf("validate /%s: %v", command.Descriptor.Name, err)
		}
		for _, identity := range command.Descriptor.identities() {
			if _, duplicate := seen[identity]; duplicate {
				t.Fatalf("command identity %q is duplicated", identity)
			}
			seen[identity] = struct{}{}
		}
		gotCategories[command.Descriptor.Category] = append(gotCategories[command.Descriptor.Category], command.Descriptor.Name)
		if got := command.Available != nil; got != wantGuard[command.Descriptor.Name] {
			t.Errorf("/%s availability policy present = %t, want %t", command.Descriptor.Name, got, wantGuard[command.Descriptor.Name])
		}
	}
	for category, want := range wantCategories {
		if got := gotCategories[category]; !slices.Equal(got, want) {
			t.Errorf("%s commands = %v, want %v", category, got, want)
		}
		delete(gotCategories, category)
	}
	for category, commands := range gotCategories {
		t.Errorf("unexpected category %q with commands %v", category, commands)
	}
}

func TestCommandCatalogRanksAnExactAliasAheadOfFuzzyNames(t *testing.T) {
	t.Parallel()
	catalog := newCommandCatalog()
	if err := catalog.add("test", CommandDescriptor{Name: "goal-resume", Title: "resume goal"}, func(string) {}, nil); err != nil {
		t.Fatal(err)
	}
	if err := catalog.add("test", CommandDescriptor{Name: "sessions", Title: "resume session", Aliases: []string{"resume"}}, func(string) {}, nil); err != nil {
		t.Fatal(err)
	}

	found := catalog.find("resume")
	if len(found) == 0 || found[0].Command.Name != "sessions" {
		t.Fatalf("find exact alias = %v, want sessions first", found)
	}
}
