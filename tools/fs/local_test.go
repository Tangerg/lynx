package fs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func skipWithoutBash(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
}

func skipWithoutGrepOrRG(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err == nil {
		return
	}
	if _, err := exec.LookPath("grep"); err != nil {
		t.Skip("neither rg nor grep available")
	}
}

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// ---------------------------------------------------------------- Read

func TestLocalExecutor_Read_Whole(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "line1\nline2\nline3\n")
	out, err := NewLocalExecutor("").Read(t.Context(), ReadInput{Path: path})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if out.TotalLines != 4 { // 3 newlines -> 4 split parts (last empty)
		t.Errorf("TotalLines = %d, want 4", out.TotalLines)
	}
	if !strings.Contains(out.Content, "line1") || !strings.Contains(out.Content, "line3") {
		t.Errorf("Content = %q, missing expected lines", out.Content)
	}
}

func TestLocalExecutor_Read_LineRange(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "a\nb\nc\nd\ne\n")
	out, err := NewLocalExecutor("").Read(t.Context(), ReadInput{
		Path:   path,
		Offset: 1, // skip "a"
		Limit:  2, // take "b", "c"
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if out.Content != "b\nc" {
		t.Errorf("Content = %q, want %q", out.Content, "b\nc")
	}
	if !out.Truncated {
		t.Error("Truncated = false, want true")
	}
}

func TestLocalExecutor_Read_MaxBytes(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "ab你cd")
	out, err := NewLocalExecutor("").Read(t.Context(), ReadInput{
		Path:     path,
		MaxBytes: 4,
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if out.Content != "ab" {
		t.Errorf("Content = %q, want %q", out.Content, "ab")
	}
	if !out.Truncated {
		t.Error("Truncated = false, want true")
	}
}

func TestLocalExecutor_Read_BinaryRejected(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "bin", "hello\x00world")
	_, err := NewLocalExecutor("").Read(t.Context(), ReadInput{Path: path})
	if !errors.Is(err, ErrBinaryFile) {
		t.Errorf("Read on binary: err = %v, want ErrBinaryFile", err)
	}
}

func TestLocalExecutor_Read_NormalizesCRLFAndBOM(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "\xEF\xBB\xBFa\r\nb\r\nc\r\n")
	out, err := NewLocalExecutor("").Read(t.Context(), ReadInput{Path: path})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if strings.Contains(out.Content, "\r") {
		t.Errorf("Content still contains \\r: %q", out.Content)
	}
	if strings.HasPrefix(out.Content, "\xEF\xBB\xBF") {
		t.Errorf("Content still has BOM prefix: %q", out.Content)
	}
}

// ---------------------------------------------------------------- Write

func TestLocalExecutor_Write_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new", "x.txt")
	out, err := NewLocalExecutor("").Write(t.Context(), WriteInput{Path: path, Content: "hi"})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if out.BytesWritten != 2 {
		t.Errorf("BytesWritten = %d, want 2", out.BytesWritten)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hi" {
		t.Errorf("file content = %q, want %q", got, "hi")
	}
}

func TestLocalExecutor_Write_Append(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "one\n")
	_, err := NewLocalExecutor("").Write(t.Context(), WriteInput{
		Path: path, Content: "two\n", Append: true,
	})
	if err != nil {
		t.Fatalf("Write append: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "one\ntwo\n" {
		t.Errorf("file content = %q, want %q", got, "one\ntwo\n")
	}
}

func TestLocalExecutor_Write_PreservesCRLF(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "old\r\ncontent\r\n")
	_, err := NewLocalExecutor("").Write(t.Context(), WriteInput{
		Path: path, Content: "new\nstuff\n",
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "\r\n") {
		t.Errorf("expected CRLF preserved, got %q", got)
	}
}

func TestLocalExecutor_Write_PreservesBOM(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "\xEF\xBB\xBFold")
	_, err := NewLocalExecutor("").Write(t.Context(), WriteInput{
		Path: path, Content: "new",
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(got), "\xEF\xBB\xBF") {
		t.Errorf("expected BOM preserved, got %q", got)
	}
}

func TestLocalExecutor_Write_NULRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	_, err := NewLocalExecutor("").Write(t.Context(), WriteInput{
		Path: path, Content: "abc\x00def",
	})
	if !errors.Is(err, ErrBinaryFile) {
		t.Errorf("Write with NUL: err = %v, want ErrBinaryFile", err)
	}
}

func TestLocalExecutor_Write_ConcurrentSamePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "race.txt")
	exec := NewLocalExecutor("")
	const N = 32
	var wg sync.WaitGroup
	for i := range N {
		wg.Go(func() {
			content := strings.Repeat("x", 1024) + "\n"
			_, err := exec.Write(t.Context(), WriteInput{Path: path, Content: content})
			if err != nil {
				t.Errorf("Write[%d]: %v", i, err)
			}
		})
	}
	wg.Wait()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Whichever write won, the result must be exactly the content of
	// one of them — no torn writes from interleaving.
	want := strings.Repeat("x", 1024) + "\n"
	if string(got) != want {
		t.Errorf("torn write detected: len=%d, want exactly %d", len(got), len(want))
	}
}

// ---------------------------------------------------------------- Edit

func TestLocalExecutor_Edit_SingleOccurrence(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "alpha beta gamma\n")
	out, err := NewLocalExecutor("").Edit(t.Context(), EditInput{
		Path: path, OldString: "beta", NewString: "BETA",
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if out.Replacements != 1 {
		t.Errorf("Replacements = %d, want 1", out.Replacements)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "alpha BETA gamma\n" {
		t.Errorf("content = %q", got)
	}
}

func TestLocalExecutor_Edit_MultipleOccurrencesRejected(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "x x x\n")
	_, err := NewLocalExecutor("").Edit(t.Context(), EditInput{
		Path: path, OldString: "x", NewString: "y",
	})
	if err == nil {
		t.Fatal("Edit with non-unique match: want error")
	}
	if !strings.Contains(err.Error(), "matches 3 times") {
		t.Errorf("err = %v, want 'matches 3 times'", err)
	}
}

func TestLocalExecutor_Edit_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "x x x\n")
	out, err := NewLocalExecutor("").Edit(t.Context(), EditInput{
		Path: path, OldString: "x", NewString: "y", ReplaceAll: true,
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if out.Replacements != 3 {
		t.Errorf("Replacements = %d, want 3", out.Replacements)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "y y y\n" {
		t.Errorf("content = %q", got)
	}
}

func TestLocalExecutor_Edit_NoMatch(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "alpha\n")
	_, err := NewLocalExecutor("").Edit(t.Context(), EditInput{
		Path: path, OldString: "beta", NewString: "BETA",
	})
	if err == nil {
		t.Fatal("Edit with no match: want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want 'not found'", err)
	}
}

func TestLocalExecutor_Edit_PreservesCRLF(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "alpha\r\nbeta\r\n")
	_, err := NewLocalExecutor("").Edit(t.Context(), EditInput{
		Path: path, OldString: "beta", NewString: "BETA",
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "alpha\r\nBETA\r\n" {
		t.Errorf("content = %q, want CRLF preserved", got)
	}
}

func TestLocalExecutor_Edit_BinaryRejected(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "bin", "hello\x00")
	_, err := NewLocalExecutor("").Edit(t.Context(), EditInput{
		Path: path, OldString: "hello", NewString: "hi",
	})
	if !errors.Is(err, ErrBinaryFile) {
		t.Errorf("Edit on binary: err = %v, want ErrBinaryFile", err)
	}
}

func TestLocalExecutor_MultiEdit_WritesOnce(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "alpha beta gamma beta\n")
	out, err := NewLocalExecutor("").MultiEdit(t.Context(), MultiEditInput{
		Path: path,
		Edits: []EditOperation{
			{OldString: "alpha", NewString: "ALPHA"},
			{OldString: "beta", NewString: "BETA", ReplaceAll: true},
		},
	})
	if err != nil {
		t.Fatalf("MultiEdit: %v", err)
	}
	if out.Edits != 2 || out.Replacements != 3 {
		t.Fatalf("MultiEdit output = %+v, want 2 edits / 3 replacements", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "ALPHA BETA gamma BETA\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestLocalExecutor_MultiEdit_FailureLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "alpha beta\n")
	_, err := NewLocalExecutor("").MultiEdit(t.Context(), MultiEditInput{
		Path: path,
		Edits: []EditOperation{
			{OldString: "alpha", NewString: "ALPHA"},
			{OldString: "missing", NewString: "MISSING"},
		},
	})
	if err == nil {
		t.Fatal("MultiEdit with failing edit: want error")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "alpha beta\n" {
		t.Fatalf("content changed despite failed multiedit: %q", got)
	}
}

// ---------------------------------------------------------------- ApplyPatch

func TestLocalExecutor_ApplyPatch_ModifyCreateDelete(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "one\ntwo\nthree\n")
	writeTemp(t, dir, "gone.txt", "remove me\n")
	patch := `diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
diff --git a/new.txt b/new.txt
--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+hello
+world
diff --git a/gone.txt b/gone.txt
--- a/gone.txt
+++ /dev/null
@@ -1 +0,0 @@
-remove me
`
	out, err := NewLocalExecutor(dir).ApplyPatch(t.Context(), ApplyPatchInput{Patch: patch})
	if err != nil {
		t.Fatalf("ApplyPatch: %v", err)
	}
	if out.Hunks != 3 || len(out.Files) != 3 {
		t.Fatalf("ApplyPatch output = %+v, want 3 hunks / files", out)
	}
	a, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(a) != "one\nTWO\nthree\n" {
		t.Fatalf("a.txt = %q", a)
	}
	newFile, _ := os.ReadFile(filepath.Join(dir, "new.txt"))
	if string(newFile) != "hello\nworld\n" {
		t.Fatalf("new.txt = %q", newFile)
	}
	if _, err := os.Stat(filepath.Join(dir, "gone.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("gone.txt still exists: %v", err)
	}
}

func TestLocalExecutor_ApplyPatch_MismatchLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "one\ntwo\n")
	patch := `--- a/a.txt
+++ b/a.txt
@@ -1,2 +1,2 @@
 one
-missing
+MISSING
`
	_, err := NewLocalExecutor(dir).ApplyPatch(t.Context(), ApplyPatchInput{Patch: patch})
	if err == nil {
		t.Fatal("ApplyPatch mismatch: want error")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "one\ntwo\n" {
		t.Fatalf("content changed despite failed patch: %q", got)
	}
}

func TestLocalExecutor_ApplyPatch_SecondFileMismatchLeavesFirstUntouched(t *testing.T) {
	dir := t.TempDir()
	first := writeTemp(t, dir, "first.txt", "one\ntwo\n")
	second := writeTemp(t, dir, "second.txt", "alpha\nbeta\n")
	patch := `--- a/first.txt
+++ b/first.txt
@@ -1,2 +1,2 @@
 one
-two
+TWO
--- a/second.txt
+++ b/second.txt
@@ -1,2 +1,2 @@
 alpha
-missing
+MISSING
`
	_, err := NewLocalExecutor(dir).ApplyPatch(t.Context(), ApplyPatchInput{Patch: patch})
	if err == nil {
		t.Fatal("ApplyPatch second-file mismatch: want error")
	}
	if got, _ := os.ReadFile(first); string(got) != "one\ntwo\n" {
		t.Fatalf("first file changed despite patch failure: %q", got)
	}
	if got, _ := os.ReadFile(second); string(got) != "alpha\nbeta\n" {
		t.Fatalf("second file changed despite patch failure: %q", got)
	}
}

func TestLocalExecutor_ApplyPatch_DuplicateFileRejectedLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "one\ntwo\nthree\n")
	patch := `--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 one
-two
+SECOND
 three
`
	_, err := NewLocalExecutor(dir).ApplyPatch(t.Context(), ApplyPatchInput{Patch: patch})
	if err == nil {
		t.Fatal("ApplyPatch duplicate file section: want error")
	}
	if !strings.Contains(err.Error(), "duplicate file patch") {
		t.Fatalf("err = %v, want duplicate file patch", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "one\ntwo\nthree\n" {
		t.Fatalf("content changed despite duplicate file patch: %q", got)
	}
}

func TestLocalExecutor_ApplyPatch_InvalidRangeRejected(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "one\n")
	patch := `--- a/a.txt
+++ b/a.txt
@@ --1,1 +1,1 @@
-one
+ONE
`
	_, err := NewLocalExecutor(dir).ApplyPatch(t.Context(), ApplyPatchInput{Patch: patch})
	if err == nil {
		t.Fatal("ApplyPatch invalid range: want error")
	}
	if !strings.Contains(err.Error(), "invalid range") {
		t.Fatalf("err = %v, want invalid range", err)
	}
}

// ---------------------------------------------------------------- Glob

func TestLocalExecutor_Glob_BasicAndDoublestar(t *testing.T) {
	skipWithoutBash(t)
	dir := t.TempDir()
	writeTemp(t, dir, "a.go", "")
	writeTemp(t, dir, "sub/b.go", "")
	writeTemp(t, dir, "sub/nested/c.go", "")
	writeTemp(t, dir, "sub/d.txt", "")

	out, err := NewLocalExecutor(dir).Glob(t.Context(), GlobInput{Pattern: "**/*.go"})
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(out.Paths) != 3 {
		t.Errorf("found %d paths, want 3: %v", len(out.Paths), out.Paths)
	}
}

func TestLocalExecutor_Glob_MaxResults(t *testing.T) {
	skipWithoutBash(t)
	dir := t.TempDir()
	for i := range 10 {
		writeTemp(t, dir, "file_"+string(rune('a'+i))+".txt", "")
	}
	out, err := NewLocalExecutor(dir).Glob(t.Context(), GlobInput{
		Pattern:    "*.txt",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(out.Paths) != 3 {
		t.Errorf("got %d paths, want 3 (capped)", len(out.Paths))
	}
	if !out.Truncated {
		t.Error("Truncated = false, want true")
	}
}

// ---------------------------------------------------------------- Grep

func TestLocalExecutor_Grep_Content(t *testing.T) {
	skipWithoutGrepOrRG(t)
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "foo bar\nbaz foo\nqux\n")
	writeTemp(t, dir, "b.txt", "no match here\n")
	out, err := NewLocalExecutor(dir).Grep(t.Context(), GrepInput{Pattern: "foo"})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if len(out.Matches) != 2 {
		t.Errorf("got %d matches, want 2: %#v", len(out.Matches), out.Matches)
	}
}

func TestLocalExecutor_Grep_FilesWithMatches(t *testing.T) {
	skipWithoutGrepOrRG(t)
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "foo\n")
	writeTemp(t, dir, "b.txt", "foo\nfoo\n")
	writeTemp(t, dir, "c.txt", "nothing\n")
	out, err := NewLocalExecutor(dir).Grep(t.Context(), GrepInput{
		Pattern:    "foo",
		OutputMode: GrepOutputFilesWithMatches,
	})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if len(out.Files) != 2 {
		t.Errorf("files = %d, want 2: %v", len(out.Files), out.Files)
	}
	if len(out.Matches) != 0 {
		t.Errorf("Matches populated in files mode: %v", out.Matches)
	}
}

func TestLocalExecutor_Grep_Count(t *testing.T) {
	skipWithoutGrepOrRG(t)
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "foo\nbar\nfoo\n")
	writeTemp(t, dir, "b.txt", "foo\n")
	writeTemp(t, dir, "c.txt", "nothing\n")
	out, err := NewLocalExecutor(dir).Grep(t.Context(), GrepInput{
		Pattern:    "foo",
		OutputMode: GrepOutputCount,
	})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if len(out.Counts) != 2 {
		t.Errorf("counts = %d, want 2 (zero-count file must be filtered): %v", len(out.Counts), out.Counts)
	}
	for _, c := range out.Counts {
		if c.Count == 0 {
			t.Errorf("count = 0 for %q, should have been filtered", c.Path)
		}
	}
}

func TestLocalExecutor_Grep_InvalidMode(t *testing.T) {
	dir := t.TempDir()
	_, err := NewLocalExecutor(dir).Grep(t.Context(), GrepInput{
		Pattern:    "foo",
		OutputMode: "bogus",
	})
	if err == nil {
		t.Fatal("Grep with bad output_mode: want error")
	}
}

func TestLocalExecutor_Grep_AsymmetricContext(t *testing.T) {
	before, after := resolveContext(GrepInput{Context: 3})
	if before != 3 || after != 3 {
		t.Errorf("Context=3 → before=%d after=%d, want 3,3", before, after)
	}
	before, after = resolveContext(GrepInput{BeforeContext: 5, AfterContext: 1})
	if before != 5 || after != 1 {
		t.Errorf("explicit B=5 A=1 → before=%d after=%d", before, after)
	}
	before, after = resolveContext(GrepInput{Context: 2, BeforeContext: 10})
	if before != 10 || after != 2 {
		t.Errorf("Context=2 + B=10 → before=%d after=%d, want 10,2 (explicit wins for before, fallback for after)", before, after)
	}
}
