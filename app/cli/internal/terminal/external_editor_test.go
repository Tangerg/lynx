package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/program"
)

func TestConfiguredDraftEditorParsesArgumentsWithoutExecutingShellSyntax(t *testing.T) {
	t.Setenv("LYRA_EDITOR", `code --wait "profile name"`)
	t.Setenv("VISUAL", "ignored")
	editor, err := configuredDraftEditor()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"code", "--wait", "profile name"}
	if strings.Join(editor.command, "|") != strings.Join(want, "|") {
		t.Fatalf("editor command = %#v", editor.command)
	}
}

func TestDraftEditorReadsSuccessfulEditsAndReportsFailure(t *testing.T) {
	workspace := t.TempDir()
	editor := &draftEditor{command: []string{"sh", "-c", `printf '\nrevised' >> "$0"`}}
	edited, err := editor.Edit(t.Context(), program.Session{}, workspace, "original")
	if err != nil {
		t.Fatal(err)
	}
	if edited != "original\nrevised" {
		t.Fatalf("edited draft = %q", edited)
	}

	failing := &draftEditor{command: []string{"sh", "-c", "exit 7"}}
	if _, err := failing.Edit(t.Context(), program.Session{}, workspace, "preserve me"); err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("failing editor error = %v", err)
	}
}

func TestDraftEditorHonorsApplicationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	editor := &draftEditor{command: []string{"sh", "-c", "sleep 30"}}
	if _, err := editor.Edit(ctx, program.Session{}, t.TempDir(), "preserve me"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled editor error = %v, want context.Canceled", err)
	}
}
