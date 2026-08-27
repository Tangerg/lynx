package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

func guardedPatchTools(dir string, format bool) (toolcontract.Tool, toolcontract.Tool) {
	tracker := newReadTracker()
	executor := fs.NewLocalExecutor(dir)
	read := withReadTracking(newRuntimeReadTool(dir, executor), tracker, dir)
	var mutation toolcontract.Tool = fs.NewApplyPatchTool(executor)
	if format {
		mutation = withAutoFormat(mutation, dir)
	}
	return read, withMutationGuard(
		withMutationRecording(withMutationDiagnostics(mutation, nil, dir)),
		tracker,
		dir,
	)
}

func patchArguments(t *testing.T, path, oldContent, newContent string) string {
	t.Helper()
	oldLines := contentLines(oldContent)
	newLines := contentLines(newContent)
	oldPath := "a/" + path
	oldStart := 1
	if oldContent == "" {
		oldPath = "/dev/null"
		oldStart = 0
	}
	newPath := "b/" + path
	if newContent == "" {
		newPath = "/dev/null"
	}
	var patch strings.Builder
	fmt.Fprintf(&patch, "--- %s\n+++ %s\n@@ -%d,%d +1,%d @@\n", oldPath, newPath, oldStart, len(oldLines), len(newLines))
	for _, line := range oldLines {
		patch.WriteByte('-')
		patch.WriteString(line)
		patch.WriteByte('\n')
	}
	for _, line := range newLines {
		patch.WriteByte('+')
		patch.WriteString(line)
		patch.WriteByte('\n')
	}
	encoded, err := json.Marshal(map[string]string{"patch": patch.String()})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func contentLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

func TestMutationGuardRequiresReadBeforeChangingExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	before := "package main\n\nfunc Foo() {}\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	_, mutation := guardedPatchTools(dir, false)
	out, err := mutation.Call(t.Context(), patchArguments(t, "foo.go", before, strings.ReplaceAll(before, "Foo", "Bar")))
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if !strings.Contains(out, "must read foo.go before modifying") {
		t.Fatalf("out = %q, want a read-first message", out)
	}
	if got, _ := os.ReadFile(path); string(got) != before {
		t.Fatalf("file changed despite guard: %q", got)
	}
}

func TestMutationRecordingReportsOnlyAppliedChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read, mutation := guardedPatchTools(dir, false)
	var recorded []string
	ctx := WithMutationRecorder(t.Context(), func(paths []string) {
		recorded = append(recorded, paths...)
	})
	arguments := patchArguments(t, "foo.txt", "before\n", "after\n")

	if _, err := mutation.Call(ctx, arguments); err != nil {
		t.Fatalf("guarded patch: %v", err)
	}
	if len(recorded) != 0 {
		t.Fatalf("guard refusal recorded mutations: %v", recorded)
	}
	if _, err := read.Call(ctx, `{"path":"foo.txt"}`); err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := mutation.Call(ctx, arguments); err != nil {
		t.Fatalf("applied patch: %v", err)
	}
	if len(recorded) != 1 || recorded[0] != "foo.txt" {
		t.Fatalf("recorded mutations = %v, want [foo.txt]", recorded)
	}
}

func TestMutationGuardRefreshesReadStampAfterPatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	before := "package main\n\nfunc Foo() {}\n"
	first := strings.ReplaceAll(before, "Foo", "Bar")
	second := strings.ReplaceAll(first, "Bar", "Baz")
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	read, mutation := guardedPatchTools(dir, false)
	if _, err := read.Call(t.Context(), `{"path":"foo.go"}`); err != nil {
		t.Fatalf("read: %v", err)
	}
	if out, err := mutation.Call(t.Context(), patchArguments(t, "foo.go", before, first)); err != nil || strings.Contains(out, "must read") {
		t.Fatalf("first patch = %q, %v", out, err)
	}
	if out, err := mutation.Call(t.Context(), patchArguments(t, "foo.go", first, second)); err != nil || strings.Contains(out, "must read") || strings.Contains(out, "changed since") {
		t.Fatalf("consecutive patch = %q, %v", out, err)
	}
	if got, _ := os.ReadFile(path); string(got) != second {
		t.Fatalf("file = %q, want %q", got, second)
	}
}

func TestMutationGuardRefreshesStampAfterAutoFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	before := `{"name":"old","count":1}` + "\n"
	first := `{"name":"new","count":1}` + "\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	read, mutation := guardedPatchTools(dir, true)
	if _, err := read.Call(t.Context(), `{"path":"data.json"}`); err != nil {
		t.Fatalf("read: %v", err)
	}
	if out, err := mutation.Call(t.Context(), patchArguments(t, "data.json", before, first)); err != nil || strings.Contains(out, "changed since") {
		t.Fatalf("first patch = %q, %v", out, err)
	}
	formatted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(formatted), "\n  \"name\": \"new\"") {
		t.Fatalf("json was not formatted: %q", formatted)
	}
	second := strings.ReplaceAll(string(formatted), "new", "newer")
	if out, err := mutation.Call(t.Context(), patchArguments(t, "data.json", string(formatted), second)); err != nil || strings.Contains(out, "changed since") {
		t.Fatalf("second patch after format = %q, %v", out, err)
	}
}

func TestMutationGuardDetectsStaleRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	before := "package main\n\nfunc Foo() {}\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	read, mutation := guardedPatchTools(dir, false)
	if _, err := read.Call(t.Context(), `{"path":"foo.go"}`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package main\n\nfunc Foo() { /* changed */ }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := mutation.Call(t.Context(), patchArguments(t, "foo.go", before, strings.ReplaceAll(before, "Foo", "Bar")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "changed since you last read it") {
		t.Fatalf("out = %q, want stale-read message", out)
	}
}

func TestMutationGuardAllowsCreatingNewFile(t *testing.T) {
	dir := t.TempDir()
	_, mutation := guardedPatchTools(dir, false)
	if out, err := mutation.Call(t.Context(), patchArguments(t, "new.txt", "", "hello\n")); err != nil || strings.Contains(out, "must read") {
		t.Fatalf("new-file patch = %q, %v", out, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "new.txt")); err != nil || string(got) != "hello\n" {
		t.Fatalf("new file = %q, %v", got, err)
	}
}

func TestMutationGuardForgetsDeletedFileBeforeExternalRecreation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.txt")
	before := "before\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	read, mutation := guardedPatchTools(dir, false)
	if _, err := read.Call(t.Context(), `{"path":"foo.txt"}`); err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := mutation.Call(t.Context(), patchArguments(t, "foo.txt", before, "")); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("external recreation: %v", err)
	}

	out, err := mutation.Call(t.Context(), patchArguments(t, "foo.txt", before, "after\n"))
	if err != nil {
		t.Fatalf("guarded patch: %v", err)
	}
	if !strings.Contains(out, "must read foo.txt before modifying") {
		t.Fatalf("out = %q, want a fresh read after external recreation", out)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != before {
		t.Fatalf("recreated file = %q, %v; mutation reused a deleted resource stamp", got, err)
	}
}

func TestFingerprintFilePreservesCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := fingerprintFile(ctx, path, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("fingerprintFile error = %v, want context.Canceled", err)
	}
}

func TestReadTrackingRejectsAFileChangedAfterReadBeforeStamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tracker := newReadTracker()
	base := fs.NewReadTool(fs.NewLocalExecutor(dir))
	readFinished := make(chan struct{})
	release := make(chan struct{})
	blocking := decorateCall(base, func(ctx context.Context, arguments string) (string, error) {
		out, err := base.Call(ctx, arguments)
		close(readFinished)
		<-release
		return out, err
	})
	tracked := withReadTracking(blocking, tracker, dir)
	done := make(chan error, 1)
	go func() {
		_, err := tracked.Call(t.Context(), `{"path":"foo.txt"}`)
		done <- err
	}()
	<-readFinished
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil || !strings.Contains(err.Error(), "changed while reading") {
		t.Fatalf("tracked read error = %v, want unstable read refusal", err)
	}
}

func TestReadTrackingRejectsSameContentReplacementDuringRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.txt")
	content := []byte("same content\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	tracker := newReadTracker()
	base := newRuntimeReadTool(dir, fs.NewLocalExecutor(dir))
	readFinished := make(chan struct{})
	release := make(chan struct{})
	blocking := decorateCall(base, func(ctx context.Context, arguments string) (string, error) {
		out, err := base.Call(ctx, arguments)
		close(readFinished)
		<-release
		return out, err
	})
	tracked := withReadTracking(blocking, tracker, dir)
	done := make(chan error, 1)
	go func() {
		_, err := tracked.Call(t.Context(), `{"path":"foo.txt"}`)
		done <- err
	}()
	<-readFinished
	replacement := filepath.Join(dir, "replacement.txt")
	if err := os.WriteFile(replacement, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil || !strings.Contains(err.Error(), "changed while reading") {
		t.Fatalf("tracked read error = %v, want same-content generation refusal", err)
	}
}

func TestReadStampAndSamePathMutationAreAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tracker := newReadTracker()
	locker := newPathLocker()
	executor := fs.NewLocalExecutor(dir)
	readStarted := make(chan struct{})
	releaseRead := make(chan struct{})
	baseRead := fs.NewReadTool(executor)
	blockingRead := decorateCall(baseRead, func(ctx context.Context, arguments string) (string, error) {
		out, err := baseRead.Call(ctx, arguments)
		close(readStarted)
		<-releaseRead
		return out, err
	})
	read := withPathLock(withReadTracking(blockingRead, tracker, dir), locker, dir)
	mutation := withPathLock(withMutationGuard(fs.NewApplyPatchTool(executor), tracker, dir), locker, dir)
	arguments := patchArguments(t, "foo.txt", "before\n", "after\n")

	readDone := make(chan error, 1)
	go func() {
		_, err := read.Call(context.Background(), `{"path":"foo.txt"}`)
		readDone <- err
	}()
	<-readStarted

	mutationDone := make(chan error, 1)
	go func() {
		_, err := mutation.Call(context.Background(), arguments)
		mutationDone <- err
	}()
	select {
	case err := <-mutationDone:
		t.Fatalf("mutation crossed active read/stamp interval: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseRead)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := <-mutationDone; err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "after\n" {
		t.Fatalf("file = %q, %v", got, err)
	}
}
