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

func TestBuiltinCommandsOwnTheirCategoryAndAvailabilityPolicy(t *testing.T) {
	t.Parallel()
	wantCategories := map[string][]string{
		commandCategoryApplication: {"quit"},
		commandCategoryTranscript:  {"help", "shortcuts", "clear", "find", "next", "previous", "queue", "details", "view", "copy-last", "export"},
		commandCategorySessions:    {"sessions", "timeline", "new", "workspace", "rename", "fork", "rollback", "import"},
		commandCategoryComposer:    {"attach", "detach", "attachments", "stash", "stashes", "stash-apply", "stash-delete", "editor"},
		commandCategoryRuntime:     {"model", "approval", "status", "rules", "steer"},
		commandCategoryWorkspace:   {"workspaces", "changes", "diff", "preview", "grep", "browse", "read"},
		commandCategoryExtensions:  {"plugins", "reload", "unload"},
	}
	wantGuard := map[string]bool{
		"clear": true, "view": true, "export": true,
		"sessions": true, "timeline": true, "new": true, "workspace": true, "rename": true, "fork": true, "rollback": true, "import": true,
		"stash": true, "stash-apply": true, "editor": true,
		"workspaces": true, "changes": true, "diff": true, "preview": true, "grep": true, "browse": true, "read": true,
		"steer": true,
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
