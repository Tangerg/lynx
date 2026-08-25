package toolset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/lynx/tool"
)

type runtimeSearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type runtimeGrepResponse struct {
	Matches   []runtimeSearchMatch `json:"matches"`
	Total     int                  `json:"total"`
	Truncated bool                 `json:"truncated"`
}

type runtimeGlobResponse struct {
	Paths     []string `json:"paths"`
	Total     int      `json:"total"`
	Truncated bool     `json:"truncated"`
}

func TestRuntimeSearchDoesNotDependOnHostSearchBinaries(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "dependency"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "dependency", "ignored.go"), []byte("package ignored\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Runtime search is an in-process product capability. Host PATH contents
	// must not decide whether the model can search a workspace.
	t.Setenv("PATH", t.TempDir())

	globBody, err := namedDirectTool(t, root, "glob").Call(t.Context(), `{"pattern":"**/*.go"}`)
	if err != nil {
		t.Fatalf("glob without host find: %v", err)
	}
	var glob runtimeGlobResponse
	if err := json.Unmarshal([]byte(globBody), &glob); err != nil {
		t.Fatalf("decode glob response: %v", err)
	}
	if len(glob.Paths) != 1 || glob.Paths[0] != "main.go" || glob.Total != 1 || glob.Truncated {
		t.Fatalf("glob response = %+v, want one ignore-aware result", glob)
	}

	grepBody, err := namedDirectTool(t, root, "grep").Call(t.Context(), `{"pattern":"package"}`)
	if err != nil {
		t.Fatalf("grep without host rg/grep: %v", err)
	}
	var grep runtimeGrepResponse
	if err := json.Unmarshal([]byte(grepBody), &grep); err != nil {
		t.Fatalf("decode grep response: %v", err)
	}
	if len(grep.Matches) != 1 || grep.Matches[0].Path != "main.go" || grep.Total != 1 || grep.Truncated {
		t.Fatalf("grep response = %+v, want one ignore-aware result", grep)
	}
}

func TestRuntimeGrepReportsExactTotalBeyondRetainedPrefix(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("needle\n", 300)
	if err := os.WriteFile(filepath.Join(root, "many.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := namedDirectTool(t, root, "grep").Call(t.Context(), `{"pattern":"needle","max_results":2}`)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	var response runtimeGrepResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode grep response: %v", err)
	}
	if len(response.Matches) != 2 || response.Total != 300 || !response.Truncated {
		t.Fatalf("grep response = {matches:%d total:%d truncated:%t}, want 2/300/true", len(response.Matches), response.Total, response.Truncated)
	}
}

func TestRuntimeGrepExcludesUnpageableTextFileAsAWhole(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pathological.txt"), []byte(strings.Repeat("needle", (1<<20)/6+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := namedDirectTool(t, root, "grep").Call(t.Context(), `{"pattern":"needle","max_results":1}`)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(body) > 64<<10 {
		t.Fatalf("grep response materialized %d bytes from an unpageable line", len(body))
	}
	var response runtimeGrepResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode grep response: %v", err)
	}
	if len(response.Matches) != 0 || response.Total != 0 || response.Truncated {
		t.Fatalf("grep response = %+v, want pathological file excluded as a whole", response)
	}
}

func TestRuntimeGrepContractContainsOnlyComposableLineSearchInputs(t *testing.T) {
	definition := namedDirectTool(t, t.TempDir(), "grep").Definition()
	schema := string(definition.InputSchema)
	for _, retired := range []string{
		`"file_type"`, `"multiline"`, `"before_context_lines"`, `"after_context_lines"`, `"output_mode"`,
	} {
		if strings.Contains(schema, retired) {
			t.Errorf("grep schema retains process-specific or overlapping field %s: %s", retired, schema)
		}
	}
	for _, required := range []string{`"pattern"`, `"path"`, `"max_results"`} {
		if !strings.Contains(schema, required) {
			t.Errorf("grep schema is missing %s: %s", required, schema)
		}
	}
}

func namedDirectTool(t *testing.T, root, name string) toolcontract.Tool {
	t.Helper()
	for _, candidate := range directTools(root) {
		if candidate.Definition().Name == name {
			return candidate
		}
	}
	t.Fatalf("direct tool %q is not registered", name)
	return nil
}
