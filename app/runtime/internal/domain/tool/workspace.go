package tool

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// BypassImmuneReason reports whether a tool call is dangerous enough to confirm
// with a human EVEN under an auto-approve mode (Yolo, or Balanced for
// write/download), returning a short reason for the approval card. immune is
// false (reason "") for an ordinary call.
//
// Two independent, deliberately-conservative checks, both DEFENSE-IN-DEPTH
// CONFIRMS — not security jails (real confinement is a sandbox executor, the
// deferred C7); they only insist a human sees the most obviously-catastrophic
// actions before an auto-approve mode runs them, and a remembered approval still
// lets a repeat through:
//   - a file mutation whose target escapes the workspace directory (a PRECISE
//     path property);
//   - a shell command matching a high-confidence catastrophic pattern
//     (rm -rf of / or $HOME, --no-preserve-root, a fork bomb, mkfs/dd to a
//     device). Tight by design so an ordinary command never trips it.
func BypassImmuneReason(name, arguments, cwd string) (reason string, immune bool) {
	if mutatesOutsideWorkspace(name, arguments, cwd) {
		return "targets a path outside the workspace directory", true
	}
	if name == "shell" && catastrophicCommand(stringArg(arguments, "command")) {
		return "runs a high-confidence catastrophic shell command (e.g. rm -rf of a root/home path, mkfs, a fork bomb)", true
	}
	return "", false
}

// mutatingPathArg maps a built-in file-mutating tool to the JSON argument that
// carries its target path. Only these tools resolve to a single filesystem
// target, so only they have a workspace-escape notion — a precise property
// (does the resolved path leave cwd). shell is handled separately by
// [catastrophicCommand] because its "path" is an opaque command line, not a
// single resolvable target.
var mutatingPathArg = map[string]string{
	"write":       "file_path",
	"edit":        "file_path",
	"apply_patch": "file_path",
	"download":    "file_path",
}

// mutatesOutsideWorkspace reports whether a file-mutating tool call's target
// path escapes the workspace directory cwd. Pure and conservative — it fails
// toward asking: a home-relative (~) target, an undecodable argument blob, or a
// path that can't be relativized all count as outside. An empty cwd (no
// workspace boundary) or a non-mutating / non-file tool is never outside.
func mutatesOutsideWorkspace(name, arguments, cwd string) bool {
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

var (
	// forkBomb matches the classic :(){ :|:& };: (whitespace-tolerant).
	forkBomb = regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`)
	// deviceDestroyers match filesystem/disk-wiping commands that are essentially
	// never run against a real target by accident.
	deviceDestroyers = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bmkfs(\.\w+)?\b`),
		regexp.MustCompile(`(?i)\bwipefs\b`),
		regexp.MustCompile(`(?i)\bdd\b[^|;&\n]*\bof=/dev/`),
		regexp.MustCompile(`(?i)>\s*/dev/(sd|nvme|hd|disk|vd)`),
	}
)

// catastrophicCommand reports whether a shell command line matches a
// high-confidence, near-zero-false-positive catastrophic pattern. Conservative
// on purpose: it flags the handful of forms that are essentially never
// intentional (rm -rf of a root/home path, the explicit --no-preserve-root, a
// fork bomb, a device wipe), and leaves everything else — including ordinary
// rm -rf of a subdirectory — alone. It is NOT a security boundary (trivially
// bypassable via quoting/variables); it is a courtesy confirm before an obvious
// disaster.
func catastrophicCommand(command string) bool {
	if command == "" {
		return false
	}
	// The explicit "yes, allow removing /" flag is catastrophic on its own.
	if strings.Contains(command, "--no-preserve-root") {
		return true
	}
	if forkBomb.MatchString(command) {
		return true
	}
	for _, re := range deviceDestroyers {
		if re.MatchString(command) {
			return true
		}
	}
	// Check each pipeline/sequence segment for a recursive-force rm of a
	// catastrophic target, so `cd x && rm -rf ~` is caught in its own segment.
	for _, segment := range shellSegments(command) {
		if recursiveForceRemoveOfRootOrHome(segment) {
			return true
		}
	}
	return false
}

// shellSegments splits a command line on the shell operators that separate
// commands (; && || | & newline) so each is inspected on its own.
func shellSegments(command string) []string {
	return regexp.MustCompile(`&&|\|\||[;|&\n]`).Split(command, -1)
}

// catastrophicRemoveTarget is the set of rm targets that a recursive-force
// delete should never hit unintentionally.
var catastrophicRemoveTarget = map[string]bool{
	"/": true, "/*": true,
	"~": true, "~/": true, "~/*": true,
	"$HOME": true, "${HOME}": true, "$HOME/": true, "$HOME/*": true,
	"*": true, ".": true, "..": true,
}

// recursiveForceRemoveOfRootOrHome reports whether one command segment is an
// `rm` invocation with BOTH recursive and force flags (any spelling / order)
// aimed at a catastrophic target.
func recursiveForceRemoveOfRootOrHome(segment string) bool {
	fields := strings.Fields(segment)
	rm := false
	recursive, force := false, false
	var targets []string
	for _, f := range fields {
		switch {
		case f == "rm" || strings.HasSuffix(f, "/rm"):
			rm = true
		case f == "sudo" || f == "command" || f == "env":
			// prefixes that still front an rm — keep scanning.
		case strings.HasPrefix(f, "--"):
			switch f {
			case "--recursive":
				recursive = true
			case "--force":
				force = true
			}
		case strings.HasPrefix(f, "-"):
			if strings.ContainsAny(f, "rR") {
				recursive = true
			}
			if strings.Contains(f, "f") {
				force = true
			}
		default:
			targets = append(targets, strings.Trim(f, `"'`))
		}
	}
	if !rm || !recursive || !force {
		return false
	}
	for _, t := range targets {
		if catastrophicRemoveTarget[t] {
			return true
		}
	}
	return false
}
