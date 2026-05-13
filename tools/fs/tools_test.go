package fs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Compile-time assertions that every tool constructor returns a value
// satisfying chat.CallableTool. (We re-assert here for documentation
// and to catch refactors that break the interface.)
func TestTools_Definitions(t *testing.T) {
	cases := []struct {
		name string
		got  string
	}{
		{"read", NewReadTool(nil).Definition().Name},
		{"write", NewWriteTool(nil).Definition().Name},
		{"edit", NewEditTool(nil).Definition().Name},
		{"glob", NewGlobTool(nil).Definition().Name},
		{"grep", NewGrepTool(nil).Definition().Name},
	}
	for _, tc := range cases {
		if tc.got != tc.name {
			t.Errorf("tool %q has Definition().Name = %q", tc.name, tc.got)
		}
	}
}

func TestReadTool_OneBasedOffsetTranslation(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "line1\nline2\nline3\nline4\n")

	tool := NewReadTool(nil)

	// offset=2 (1-based) means "start at line 2"; limit=2 takes line2,line3
	body, err := tool.Call(t.Context(), `{"path":"`+path+`","offset":2,"limit":2}`)
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

func TestReadTool_OffsetZeroMeansStart(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "a.txt", "a\nb\nc\n")
	body, err := NewReadTool(nil).Call(t.Context(), `{"path":"`+path+`","offset":0,"limit":1}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp ReadResponse
	_ = json.Unmarshal([]byte(body), &resp)
	if resp.StartLine != 1 {
		t.Errorf("StartLine = %d, want 1 (offset=0 → start at first line)", resp.StartLine)
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

func TestGrepTool_ContentMode(t *testing.T) {
	skipWithoutGrepOrRG(t)
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
	if len(resp.Matches) == 0 {
		t.Errorf("no matches in body=%s", body)
	}
}

func TestGrepTool_FilesWithMatchesMode(t *testing.T) {
	skipWithoutGrepOrRG(t)
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
	if len(resp.Matches) != 0 {
		t.Errorf("matches must be empty in files mode: %v", resp.Matches)
	}
	// JSON sum-type sanity: matches/counts must be absent (omitempty)
	if strings.Contains(body, `"matches"`) {
		t.Errorf("body should omit matches in files mode; got %s", body)
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
