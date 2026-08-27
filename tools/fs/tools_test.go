package fs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

type fileMutationReporter interface {
	MutationPaths(arguments string) ([]string, error)
}

// Compile-time assertions that every tool constructor returns a value
// satisfying chat.Tool. (We re-assert here for documentation
// and to catch refactors that break the interface.)
func TestTools_Definitions(t *testing.T) {
	cases := []struct {
		name string
		got  string
	}{
		{"read", NewReadTool(nil).Definition().Name},
		{"write", NewWriteTool(nil).Definition().Name},
		{"edit", NewEditTool(nil).Definition().Name},
		{"apply_patch", NewApplyPatchTool(nil).Definition().Name},
		{"glob", NewGlobTool(nil).Definition().Name},
		{"grep", NewGrepTool(nil).Definition().Name},
	}
	for _, tc := range cases {
		if tc.got != tc.name {
			t.Errorf("tool %q has Definition().Name = %q", tc.name, tc.got)
		}
	}
}

func TestToolContractsUseOneStrictFileVocabulary(t *testing.T) {
	for _, candidate := range []struct {
		name string
		tool interface {
			Definition() chat.ToolDefinition
			Call(context.Context, string) (string, error)
		}
		removedArguments string
	}{
		{"read", NewReadTool(nil), `{"file_path":"old.txt"}`},
		{"write", NewWriteTool(nil), `{"path":"new.txt","content":"x","append":true}`},
		{"edit", NewEditTool(nil), `{"file_path":"old.txt","old_string":"a","new_string":"b"}`},
		{"apply_patch", NewApplyPatchTool(nil), `{"patch":"x","unknown":true}`},
		{"glob", NewGlobTool(nil), `{"pattern":"**/*","max_results":1001}`},
		{"grep", NewGrepTool(nil), `{"pattern":"x","head_limit":1}`},
	} {
		t.Run(candidate.name, func(t *testing.T) {
			definition := candidate.tool.Definition()
			if strings.Contains(string(definition.InputSchema), `"file_path"`) {
				t.Fatalf("schema still exposes file_path: %s", definition.InputSchema)
			}
			if _, err := candidate.tool.Call(t.Context(), candidate.removedArguments); err == nil {
				t.Fatalf("accepted removed or out-of-range arguments: %s", candidate.removedArguments)
			}
		})
	}
}

func TestGrepContractRejectsAmbiguousPagingAndContextFields(t *testing.T) {
	grep := NewGrepTool(nil)
	for _, arguments := range []string{
		`{"pattern":"x","glob":"**/*.go"}`,
		`{"pattern":"x","type":"go"}`,
		`{"pattern":"x","context":2}`,
		`{"pattern":"x","before_context":2}`,
		`{"pattern":"x","after_context":2}`,
		`{"pattern":"x","output_mode":"paths"}`,
		`{"pattern":"x","before_context_lines":21}`,
	} {
		if _, err := grep.Call(t.Context(), arguments); err == nil {
			t.Fatalf("grep accepted invalid arguments: %s", arguments)
		}
	}
}

func TestFileToolsReportMutationPaths(t *testing.T) {
	tests := []struct {
		name      string
		tool      fileMutationReporter
		arguments string
		want      []string
	}{
		{"write", NewWriteTool(nil), `{"path":"a.go"}`, []string{"a.go"}},
		{"edit", NewEditTool(nil), `{"path":"b.go"}`, []string{"b.go"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.tool.MutationPaths(test.arguments)
			if err != nil {
				t.Fatalf("MutationPaths: %v", err)
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("MutationPaths = %v, want %v", got, test.want)
			}
		})
	}
}

func TestReadTool_OneBasedStartLineTranslation(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "line1\nline2\nline3\nline4\n")

	tool := NewReadTool(nil)

	// start_line=2 means "start at line 2"; max_lines=2 takes line2,line3.
	body, err := tool.Call(t.Context(), `{"path":"`+path+`","start_line":2,"max_lines":2}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp ReadResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal: %v body=%s", err, body)
	}
	if resp.Content != "line2\nline3" {
		t.Errorf("Content = %q, want %q", resp.Content, "line2\nline3")
	}
	if resp.StartLine != 2 {
		t.Errorf("StartLine = %d, want 2 (1-based)", resp.StartLine)
	}
	if resp.EndLine != 3 {
		t.Errorf("EndLine = %d, want 3 (1-based inclusive)", resp.EndLine)
	}
}

func TestReadTool_OmittedStartLineMeansFirstLine(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "a\nb\nc\n")
	body, err := NewReadTool(nil).Call(t.Context(), `{"path":"`+path+`","max_lines":1}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp ReadResponse
	_ = json.Unmarshal([]byte(body), &resp)
	if resp.StartLine != 1 {
		t.Errorf("StartLine = %d, want 1 when start_line is omitted", resp.StartLine)
	}
}

func TestReadTool_RejectsAmbiguousLegacyPaging(t *testing.T) {
	for _, arguments := range []string{
		`{"path":"a.txt","offset":1}`,
		`{"path":"a.txt","limit":20}`,
		`{"path":"a.txt","start_line":0}`,
	} {
		if _, err := NewReadTool(nil).Call(t.Context(), arguments); err == nil {
			t.Fatalf("read accepted ambiguous paging arguments: %s", arguments)
		}
	}
}

func TestReadTool_EmptyPath(t *testing.T) {
	_, err := NewReadTool(nil).Call(t.Context(), `{"path":""}`)
	if err == nil {
		t.Fatal("Call with empty path: want error")
	}
}

func TestWriteTool_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	body, err := NewWriteTool(nil).Call(t.Context(), `{"path":"`+path+`","content":"hi"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp WriteResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if resp.BytesWritten != 2 {
		t.Errorf("BytesWritten = %d, want 2", resp.BytesWritten)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hi" {
		t.Errorf("file = %q, want %q", got, "hi")
	}
}

func TestEditTool_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "alpha beta\n")
	body, err := NewEditTool(nil).Call(t.Context(),
		`{"path":"`+path+`","old_string":"beta","new_string":"BETA"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp EditResponse
	_ = json.Unmarshal([]byte(body), &resp)
	if resp.Replacements != 1 {
		t.Errorf("Replacements = %d, want 1", resp.Replacements)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "alpha BETA\n" {
		t.Errorf("file = %q", got)
	}
}

func TestApplyPatchTool_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "alpha\nbeta\n")
	patch := `--- ` + path + `
+++ ` + path + `
@@ -1,2 +1,2 @@
 alpha
-beta
+BETA
`
	body, err := NewApplyPatchTool(nil).Call(t.Context(), string(mustJSON(t, ApplyPatchRequest{Patch: patch})))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp ApplyPatchResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if resp.Hunks != 1 || len(resp.Files) != 1 {
		t.Fatalf("response = %+v, want one file/hunk", resp)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "alpha\nBETA\n" {
		t.Errorf("file = %q", got)
	}
}

func TestGrepTool_ContentMode(t *testing.T) {
	skipWithoutRipgrep(t)
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "foo bar\n")
	body, err := NewGrepTool(NewLocalExecutor(dir)).Call(t.Context(), `{"pattern":"foo"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp GrepResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal: %v body=%s", err, body)
	}
	if len(resp.Lines) == 0 {
		t.Errorf("no matches in body=%s", body)
	}
}

func TestGrepTool_FilesWithMatchesMode(t *testing.T) {
	skipWithoutRipgrep(t)
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "foo\n")
	writeTemp(t, dir, "b.txt", "bar\n")
	body, err := NewGrepTool(NewLocalExecutor(dir)).Call(t.Context(),
		`{"pattern":"foo","output_mode":"files_with_matches"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp GrepResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal: %v body=%s", err, body)
	}
	if len(resp.Files) == 0 {
		t.Errorf("expected files populated; body=%s", body)
	}
	if len(resp.Lines) != 0 {
		t.Errorf("lines must be empty in files mode: %v", resp.Lines)
	}
	// JSON sum-type sanity: lines/counts must be absent (omitempty)
	if strings.Contains(body, `"lines"`) {
		t.Errorf("body should omit lines in files mode; got %s", body)
	}
}

func TestGlobTool_Description(t *testing.T) {
	def := NewGlobTool(nil).Definition()
	for _, kw := range []string{"**/*.go", "doublestar", "grep"} {
		if !strings.Contains(def.Description, kw) {
			t.Errorf("Description missing %q: %q", kw, def.Description)
		}
	}
}

func TestGrepTool_Description(t *testing.T) {
	def := NewGrepTool(nil).Definition()
	for _, kw := range []string{"ripgrep", "multiline", "files_with_matches"} {
		if !strings.Contains(def.Description, kw) {
			t.Errorf("Description missing %q: %q", kw, def.Description)
		}
	}
}

func TestBadJSONArguments(t *testing.T) {
	tools := []struct {
		name string
		call func(ctx context.Context, args string) (string, error)
	}{
		{"read", NewReadTool(nil).Call},
		{"write", NewWriteTool(nil).Call},
		{"edit", NewEditTool(nil).Call},
		{"apply_patch", NewApplyPatchTool(nil).Call},
		{"glob", NewGlobTool(nil).Call},
		{"grep", NewGrepTool(nil).Call},
	}
	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.call(t.Context(), `{not json`); err == nil {
				t.Errorf("%s tool: want error on bad JSON", tc.name)
			}
		})
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return b
}
