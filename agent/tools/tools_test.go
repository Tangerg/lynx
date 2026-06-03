package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/tools"
	"github.com/Tangerg/lynx/core/model/chat"
)

func toolByName(ts []chat.Tool, name string) chat.Tool {
	for _, t := range ts {
		if t.Definition().Name == name {
			return t
		}
	}
	return nil
}

func TestMathTools(t *testing.T) {
	ts := tools.MathTools()
	ctx := context.Background()

	cases := []struct {
		tool string
		args string
		want string
	}{
		{"add", `{"a":2,"b":3}`, "5"},
		{"subtract", `{"a":5,"b":3}`, "2"},
		{"multiply", `{"a":4,"b":2.5}`, "10"},
		{"divide", `{"a":9,"b":3}`, "3"},
	}
	for _, tc := range cases {
		tool := toolByName(ts, tc.tool)
		if tool == nil {
			t.Fatalf("tool %q not found", tc.tool)
		}
		got, err := tool.Call(ctx, tc.args)
		if err != nil {
			t.Fatalf("%s: %v", tc.tool, err)
		}
		if got != tc.want {
			t.Errorf("%s(%s) = %q, want %q", tc.tool, tc.args, got, tc.want)
		}
	}
}

func TestMathTools_DivideByZero(t *testing.T) {
	div := toolByName(tools.MathTools(), "divide")
	if _, err := div.Call(context.Background(), `{"a":1,"b":0}`); err == nil {
		t.Fatal("expected division-by-zero error")
	}
}

func TestFileTools_ReadAndList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	ts, err := tools.FileTools(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	read := toolByName(ts, "read_file")
	got, err := read.Call(ctx, `{"path":"a.txt"}`)
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if got != "hello" {
		t.Fatalf("read_file = %q, want hello", got)
	}

	list := toolByName(ts, "list_files")
	out, err := list.Call(ctx, `{"pattern":"*.txt"}`)
	if err != nil {
		t.Fatalf("list_files: %v", err)
	}
	if out != "a.txt\nb.txt" {
		t.Fatalf("list_files = %q, want \"a.txt\\nb.txt\"", out)
	}
}

func TestFileTools_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	ts, err := tools.FileTools(dir)
	if err != nil {
		t.Fatal(err)
	}
	read := toolByName(ts, "read_file")

	for _, bad := range []string{`{"path":"../../etc/passwd"}`, `{"path":"/etc/passwd"}`} {
		if _, err := read.Call(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "escape") {
			t.Errorf("read_file(%s): want escape error, got %v", bad, err)
		}
	}
}

func TestFileTools_EmptyDirErrors(t *testing.T) {
	if _, err := tools.FileTools(""); err == nil {
		t.Fatal("expected error for empty dir")
	}
}
