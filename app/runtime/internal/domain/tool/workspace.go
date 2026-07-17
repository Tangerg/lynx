package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// mutatingPathArg maps a built-in file-mutating tool to the JSON argument that
// carries its target path. Only these tools resolve to a single filesystem
// target, so only they have a workspace-escape notion here. shell is
// deliberately absent: its "path" is an opaque command line, not one resolvable
// target — classifying a destructive command is a separate concern, not this
// path check.
var mutatingPathArg = map[string]string{
	"write":       "file_path",
	"edit":        "file_path",
	"apply_patch": "file_path",
	"download":    "file_path",
}

// MutatesOutsideWorkspace reports whether a file-mutating tool call's target
// path escapes the workspace directory cwd. This is the operation that must be
// confirmed even under an auto-approve mode: the workspace is the trust
// boundary, so a write beyond it is exactly what a blanket "approve everything"
// should not silently cover (see [approval.ToolCallInput.Cwd]).
//
// Pure and conservative — it fails toward asking: a home-relative (~) target, an
// undecodable argument blob, or a path that can't be relativized all count as
// outside. An empty cwd (no workspace boundary configured) or a non-mutating /
// non-file tool is never outside.
func MutatesOutsideWorkspace(name, arguments, cwd string) bool {
	arg, ok := mutatingPathArg[name]
	if !ok || cwd == "" {
		return false
	}
	target := stringArg(arguments, arg)
	if target == "" {
		return false
	}
	return escapesDir(target, cwd)
}

// stringArg pulls a single string field out of a tool's JSON arguments, or ""
// when it is absent, not a string, or the blob doesn't decode.
func stringArg(arguments, key string) string {
	var fields map[string]any
	if json.Unmarshal([]byte(arguments), &fields) != nil {
		return ""
	}
	value, _ := fields[key].(string)
	return value
}

// escapesDir reports whether path resolves outside dir. A ~-prefixed target is
// home-relative and treated as outside (the workspace is the project dir, not
// home); a relative target is anchored under dir; an absolute target is compared
// directly. Rune/separator-safe via filepath.Rel.
func escapesDir(path, dir string) bool {
	if strings.HasPrefix(path, "~") {
		return true
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
