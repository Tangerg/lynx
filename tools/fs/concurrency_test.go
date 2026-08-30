package fs

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// concurrencyAware is the optional scheduling contract the tool loop discovers
// on a tool. It is declared here rather than imported so this package keeps
// depending only on what it uses.
type concurrencyAware interface {
	ConcurrencyKey(invocation toolcontract.Invocation) (key string, concurrent bool)
}

func invocationFor(t *testing.T, executable toolcontract.Tool, arguments string) toolcontract.Invocation {
	t.Helper()
	binding, err := toolcontract.Bind(executable)
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := binding.Prepare(chat.ToolCall{
		ID: "test-call", Name: binding.Definition().Name, Arguments: arguments,
	})
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}

// TestReadOnlyToolsDeclareNoConflict is the scheduling contract the tool loop
// relies on: a read has no local resource to conflict on, so parallel reads
// must never be serialized behind an accidental key.
func TestReadOnlyToolsDeclareNoConflict(t *testing.T) {
	root := t.TempDir()
	executor := NewLocalExecutor(root)

	cases := map[string]struct {
		tool      toolcontract.Tool
		arguments string
	}{
		"read": {tool: NewReadTool(executor), arguments: `{"path":"a.txt"}`},
		"glob": {tool: NewGlobTool(executor), arguments: `{"pattern":"**/*.go"}`},
		"grep": {tool: NewGrepTool(executor), arguments: `{"pattern":"scope"}`},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			aware, ok := testCase.tool.(concurrencyAware)
			if !ok {
				t.Fatalf("%T does not declare a concurrency key", testCase.tool)
			}
			key, concurrent := aware.ConcurrencyKey(invocationFor(t, testCase.tool, testCase.arguments))
			if !concurrent {
				t.Fatal("a read-only tool declared itself exclusive")
			}
			if key != "" {
				t.Fatalf("a read-only tool claimed the conflict key %q", key)
			}
		})
	}
}

// TestMutatingToolsConflictOnTheirTargetPath is the other half of the contract:
// two edits to the same file must serialize, and edits to different files must
// not. The key is the path, so the loop can decide without understanding the
// tool.
func TestMutatingToolsConflictOnTheirTargetPath(t *testing.T) {
	root := t.TempDir()
	executor := NewLocalExecutor(root)

	tools := map[string]toolcontract.Tool{
		"edit":  NewEditTool(executor),
		"write": NewWriteTool(executor),
	}
	for name, executable := range tools {
		t.Run(name, func(t *testing.T) {
			aware, ok := executable.(concurrencyAware)
			if !ok {
				t.Fatalf("%T does not declare a concurrency key", executable)
			}

			first, concurrent := aware.ConcurrencyKey(invocationFor(t, executable, arguments(name, "a.txt")))
			if !concurrent {
				t.Fatal("a mutating tool declared itself globally exclusive")
			}
			if first != "a.txt" {
				t.Fatalf("conflict key = %q, want the target path", first)
			}

			same, _ := aware.ConcurrencyKey(invocationFor(t, executable, arguments(name, "a.txt")))
			if same != first {
				t.Fatalf("the same path produced two keys: %q and %q", first, same)
			}

			other, _ := aware.ConcurrencyKey(invocationFor(t, executable, arguments(name, "b.txt")))
			if other == first {
				t.Fatalf("distinct paths shared the conflict key %q", other)
			}
		})
	}
}

func arguments(tool, path string) string {
	if tool == "edit" {
		return fmt.Sprintf(`{"path":%q,"old_string":"a","new_string":"b"}`, path)
	}
	return fmt.Sprintf(`{"path":%q,"content":"body"}`, path)
}

// TestGrepLineKindIsAClosedVocabulary keeps the structured grep event readable:
// a kind outside the pair would leave a consumer unable to tell a match from
// requested context.
func TestGrepLineKindIsAClosedVocabulary(t *testing.T) {
	for _, kind := range []GrepLineKind{GrepLineMatch, GrepLineContext} {
		if !kind.Valid() {
			t.Errorf("%q reports itself invalid", kind)
		}
		if kind.String() != string(kind) {
			t.Errorf("%q prints as %q", kind, kind.String())
		}
	}
	for _, kind := range []GrepLineKind{"", "unknown", "MATCH"} {
		if kind.Valid() {
			t.Errorf("%q reports itself valid", kind)
		}
	}
}

// TestReadLineNumberSurfacesTheOffendingLine is what turns an oversized-line
// failure into an actionable one: without the line number the caller only knows
// that some line in the file was too long.
func TestReadLineNumberSurfacesTheOffendingLine(t *testing.T) {
	err := &lineLimitError{path: "a.txt", line: 42, limit: 1024}
	if !errors.Is(err, ErrLineTooLarge) {
		t.Fatalf("error does not unwrap to ErrLineTooLarge: %v", err)
	}
	if got := ReadLineNumber(err); got != 42 {
		t.Fatalf("ReadLineNumber = %d, want 42", got)
	}
	if message := err.Error(); message == "" {
		t.Fatal("the error has no message")
	}

	if got := ReadLineNumber(ErrFileTooLarge); got != 0 {
		t.Fatalf("ReadLineNumber on a non-line error = %d, want 0", got)
	}
	if got := ReadLineNumber(nil); got != 0 {
		t.Fatalf("ReadLineNumber(nil) = %d, want 0", got)
	}
}
