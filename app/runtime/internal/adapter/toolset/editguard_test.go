package toolset

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/editguard"
	"github.com/Tangerg/lynx/tools/fs"
)

// guardTools builds the read/edit/write tools wrapped with the read-tracker
// guards over dir. The tests pass no code-intelligence analyzer, so the
// diagnostics wrap is a no-op. They drive the tools with a plain context, so
// turnSession resolves to "" and every call shares one session bucket.
func guardTools(dir string) (read, edit, write chat.Tool, tr *editguard.Tracker) {
	tr = editguard.NewTracker()
	ex := fs.NewLocalExecutor(dir)
	read = withReadTracking(fs.NewReadTool(ex), tr, dir)
	edit = withEditGuard(withEditDiagnostics(fs.NewEditTool(ex), nil, dir), tr, dir)
	write = withWriteGuard(withEditDiagnostics(fs.NewWriteTool(ex), nil, dir), tr, dir)
	return read, edit, write, tr
}

func guardToolsWithFormat(dir string) (read, edit chat.Tool) {
	tr := editguard.NewTracker()
	ex := fs.NewLocalExecutor(dir)
	read = withReadTracking(fs.NewReadTool(ex), tr, dir)
	edit = withEditGuard(withEditDiagnostics(withAutoFormat(fs.NewEditTool(ex), dir), nil, dir), tr, dir)
	return read, edit
}

func TestEditGuard_RequiresReadFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, edit, _, _ := guardTools(dir)
	ctx := context.Background()

	// Edit before any read → guided to read first, file untouched.
	out, err := edit.Call(ctx, `{"file_path":"foo.go","old_string":"Foo","new_string":"Bar"}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "must read foo.go before editing") {
		t.Fatalf("out = %q, want a read-first message", out)
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "Bar") {
		t.Fatal("edit applied despite the read-first guard")
	}
}

func TestEditGuard_ReadThenEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	os.WriteFile(path, []byte("package main\n\nfunc Foo() {}\n"), 0o644)
	read, edit, _, _ := guardTools(dir)
	ctx := context.Background()

	if _, err := read.Call(ctx, `{"file_path":"foo.go"}`); err != nil {
		t.Fatalf("read: %v", err)
	}
	out, err := edit.Call(ctx, `{"file_path":"foo.go","old_string":"Foo","new_string":"Bar"}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if strings.Contains(out, "must read") || strings.Contains(out, "changed since") {
		t.Fatalf("edit after read was blocked: %q", out)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "Bar") {
		t.Fatal("edit did not apply")
	}

	// A second edit without re-reading works — the stamp was refreshed.
	if out, err := edit.Call(ctx, `{"file_path":"foo.go","old_string":"Bar","new_string":"Baz"}`); err != nil || strings.Contains(out, "must read") || strings.Contains(out, "changed since") {
		t.Fatalf("consecutive edit blocked: out=%q err=%v", out, err)
	}
}

func TestEditGuard_RefreshesAfterAutoFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte(`{"name":"old","count":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	read, edit := guardToolsWithFormat(dir)
	ctx := context.Background()

	if _, err := read.Call(ctx, `{"file_path":"data.json"}`); err != nil {
		t.Fatalf("read: %v", err)
	}
	if out, err := edit.Call(ctx, `{"file_path":"data.json","old_string":"old","new_string":"new"}`); err != nil || strings.Contains(out, "changed since") {
		t.Fatalf("first edit blocked: out=%q err=%v", out, err)
	}
	formatted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(formatted), "\n  \"name\": \"new\"") {
		t.Fatalf("json was not formatted after edit: %q", formatted)
	}

	if out, err := edit.Call(ctx, `{"file_path":"data.json","old_string":"new","new_string":"newer"}`); err != nil || strings.Contains(out, "changed since") {
		t.Fatalf("second edit after auto-format was blocked: out=%q err=%v", out, err)
	}
}

func TestEditGuard_StaleDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	os.WriteFile(path, []byte("package main\n\nfunc Foo() {}\n"), 0o644)
	read, edit, _, _ := guardTools(dir)
	ctx := context.Background()

	if _, err := read.Call(ctx, `{"file_path":"foo.go"}`); err != nil {
		t.Fatalf("read: %v", err)
	}
	// Someone else rewrites the file after the read.
	os.WriteFile(path, []byte("package main\n\nfunc Foo() { /* edited */ }\n"), 0o644)

	out, err := edit.Call(ctx, `{"file_path":"foo.go","old_string":"Foo","new_string":"Bar"}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "changed since you last read it") {
		t.Fatalf("out = %q, want a stale-file message", out)
	}
}

func TestWriteGuard_NewFileExemptOverwriteGuarded(t *testing.T) {
	dir := t.TempDir()
	_, _, write, _ := guardTools(dir)
	ctx := context.Background()

	// New file: no prior read required.
	if out, err := write.Call(ctx, `{"file_path":"new.txt","content":"hello"}`); err != nil || strings.Contains(out, "must read") {
		t.Fatalf("new-file write blocked: out=%q err=%v", out, err)
	}

	// Overwriting an existing file without reading it → guided to read first.
	existing := filepath.Join(dir, "old.txt")
	os.WriteFile(existing, []byte("original"), 0o644)
	out, err := write.Call(ctx, `{"file_path":"old.txt","content":"clobbered"}`)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out, "must read old.txt before overwriting") {
		t.Fatalf("out = %q, want an overwrite read-first message", out)
	}
	if b, _ := os.ReadFile(existing); string(b) != "original" {
		t.Fatal("overwrite applied despite the guard")
	}
}
