package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/lynx/core/tool"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
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
	if unmarshalErr := json.Unmarshal([]byte(globBody), &glob); unmarshalErr != nil {
		t.Fatalf("decode glob response: %v", unmarshalErr)
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

func TestRuntimeGlobReportsExactTotalBeyondRetainedPrefix(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	body, err := namedDirectTool(t, root, "glob").Call(t.Context(), `{"pattern":"*.go","max_results":2}`)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	var response runtimeGlobResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode glob response: %v", err)
	}
	if len(response.Paths) != 2 || response.Total != 3 || !response.Truncated {
		t.Fatalf("glob response = {paths:%d total:%d truncated:%t}, want 2/3/true", len(response.Paths), response.Total, response.Truncated)
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

func TestRuntimeGrepBoundsEncodedToolResult(t *testing.T) {
	root := t.TempDir()
	line := strings.Repeat("\x01", 2<<10) + "needle\n"
	if err := os.WriteFile(filepath.Join(root, "escaped.txt"), []byte(strings.Repeat(line, 1000)), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := namedDirectTool(t, root, "grep").Call(t.Context(), `{"pattern":"needle","max_results":1000}`)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(body) > 1<<20 {
		t.Fatalf("encoded grep response = %d bytes, want at most 1 MiB", len(body))
	}
	var response runtimeGrepResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode grep response: %v", err)
	}
	if response.Total != 1000 || !response.Truncated || len(response.Matches) == 0 || len(response.Matches) >= response.Total {
		t.Fatalf("grep response = {matches:%d total:%d truncated:%t}, want bounded non-empty prefix of 1000", len(response.Matches), response.Total, response.Truncated)
	}
}

func TestRuntimeSearchContractsContainOnlyComposableInputs(t *testing.T) {
	definition := namedDirectTool(t, t.TempDir(), "grep").Definition()
	schema := string(definition.InputSchema)
	for _, retired := range []string{
		`"file_glob"`, `"file_type"`, `"ignore_case"`, `"multiline"`,
		`"before_context_lines"`, `"after_context_lines"`, `"output_mode"`,
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
	globSchema := string(namedDirectTool(t, t.TempDir(), "glob").Definition().InputSchema)
	if strings.Contains(globSchema, `"ignore_case"`) {
		t.Errorf("glob schema retains overlapping field ignore_case: %s", globSchema)
	}
	for _, required := range []string{`"pattern"`, `"path"`, `"max_results"`} {
		if !strings.Contains(globSchema, required) {
			t.Errorf("glob schema is missing %s: %s", required, globSchema)
		}
	}
}

func TestRuntimeSearchConfinesPathsAndPreservesCancellation(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	for _, name := range []string{"glob", "grep"} {
		t.Run(name+" lexical escape", func(t *testing.T) {
			arguments := `{"pattern":"*","path":"../"}`
			if name == "grep" {
				arguments = `{"pattern":"secret","path":"../"}`
			}
			_, err := namedDirectTool(t, root, name).Call(t.Context(), arguments)
			if !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
				t.Fatalf("%s error = %v, want ErrPathOutsideRoot", name, err)
			}
		})
		t.Run(name+" symlink escape", func(t *testing.T) {
			arguments := `{"pattern":"*","path":"outside"}`
			if name == "grep" {
				arguments = `{"pattern":"secret","path":"outside"}`
			}
			_, err := namedDirectTool(t, root, name).Call(t.Context(), arguments)
			if !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
				t.Fatalf("%s error = %v, want ErrPathOutsideRoot", name, err)
			}
		})
		t.Run(name+" canceled", func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := namedDirectTool(t, root, name).Call(ctx, `{"pattern":"*"}`)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s error = %v, want context.Canceled", name, err)
			}
		})
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
