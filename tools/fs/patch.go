package fs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

var errRenamePatch = errors.New("fs: patch renames are not supported")

type unifiedPatch struct {
	files []filePatch
}

type filePatch struct {
	oldPath string
	newPath string
	hunks   []patchHunk
}

func (p filePatch) path() string {
	if p.newPath != "" && p.newPath != "/dev/null" {
		return p.newPath
	}
	return p.oldPath
}

func (p filePatch) created() bool { return p.oldPath == "/dev/null" }
func (p filePatch) deleted() bool { return p.newPath == "/dev/null" }

type patchHunk struct {
	oldStart int
	oldCount int
	newStart int
	newCount int
	lines    []patchLine
}

type patchLine struct {
	kind byte
	text string
}

func patchPaths(patch string) ([]string, error) {
	parsed, err := parseUnifiedPatch(patch)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(parsed.files))
	for _, file := range parsed.files {
		paths = append(paths, file.path())
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

func (l *LocalExecutor) ApplyPatch(_ context.Context, in ApplyPatchInput) (ApplyPatchOutput, error) {
	parsed, err := parseUnifiedPatch(in.Patch)
	if err != nil {
		return ApplyPatchOutput{}, err
	}

	resolved := make([]string, len(parsed.files))
	for i, file := range parsed.files {
		if err := validatePatchFile(file); err != nil {
			return ApplyPatchOutput{}, err
		}
		path, err := l.resolve(file.path())
		if err != nil {
			return ApplyPatchOutput{}, err
		}
		resolved[i] = path
	}

	for _, path := range sortedUnique(resolved) {
		unlock := l.lockPath(path)
		defer unlock()
	}

	prepared := make([]preparedPatch, len(parsed.files))
	for i, file := range parsed.files {
		next, err := l.preparePatch(file, resolved[i])
		if err != nil {
			return ApplyPatchOutput{}, err
		}
		prepared[i] = next
	}

	var out ApplyPatchOutput
	for _, file := range prepared {
		if err := file.commit(); err != nil {
			return ApplyPatchOutput{}, err
		}
		out.Files = append(out.Files, file.result)
		out.Hunks += file.result.Hunks
	}
	return out, nil
}

func validatePatchFile(file filePatch) error {
	if len(file.hunks) == 0 {
		return errors.New("fs.ApplyPatch: file patch has no hunks")
	}
	if file.oldPath == "" || file.newPath == "" {
		return errors.New("fs.ApplyPatch: file patch is missing ---/+++ headers")
	}
	if file.oldPath != "/dev/null" && file.newPath != "/dev/null" && file.oldPath != file.newPath {
		return fmt.Errorf("fs.ApplyPatch: %w: %s -> %s", errRenamePatch, file.oldPath, file.newPath)
	}
	return nil
}

type preparedPatch struct {
	path   string
	data   []byte
	mode   os.FileMode
	remove bool
	result PatchFileOutput
}

func (p preparedPatch) commit() error {
	if p.remove {
		return os.Remove(p.path)
	}
	return atomicWriteFile(p.path, p.data, p.mode)
}

func (l *LocalExecutor) preparePatch(file filePatch, path string) (preparedPatch, error) {
	if file.created() {
		if _, err := os.Stat(path); err == nil {
			return preparedPatch{}, fmt.Errorf("fs.ApplyPatch: create %s: file already exists", file.path())
		}
	}

	mode := os.FileMode(0o644)
	var lines []string
	hadBOM, hadCRLF := false, false
	if !file.created() {
		info, err := os.Stat(path)
		if err != nil {
			return preparedPatch{}, err
		}
		mode = info.Mode().Perm()
		data, err := os.ReadFile(path)
		if err != nil {
			return preparedPatch{}, err
		}
		if looksBinary(data) {
			return preparedPatch{}, ErrBinaryFile
		}
		text, bom, crlf := normalizeText(data)
		hadBOM, hadCRLF = bom, crlf
		lines = splitTextLines(text)
	}

	patched, err := applyHunks(lines, file.hunks, file.path())
	if err != nil {
		return preparedPatch{}, err
	}
	if file.deleted() {
		if len(patched) != 0 {
			return preparedPatch{}, fmt.Errorf("fs.ApplyPatch: delete %s: patched content is not empty", file.path())
		}
		return preparedPatch{
			path:   path,
			remove: true,
			result: PatchFileOutput{Path: file.path(), Hunks: len(file.hunks), Deleted: true},
		}, nil
	}

	data := restoreFormat(joinTextLines(patched), hadBOM, hadCRLF)
	return preparedPatch{
		path: path,
		data: data,
		mode: mode,
		result: PatchFileOutput{
			Path:    file.path(),
			Hunks:   len(file.hunks),
			Created: file.created(),
		},
	}, nil
}

func applyHunks(lines []string, hunks []patchHunk, path string) ([]string, error) {
	out := slices.Clone(lines)
	delta := 0
	for _, hunk := range hunks {
		oldLines, newLines := hunkLines(hunk)
		idx := hunk.oldStart - 1 + delta
		if hunk.oldStart == 0 {
			idx = delta
		}
		if idx < 0 || idx+len(oldLines) > len(out) || !equalLines(out[idx:idx+len(oldLines)], oldLines) {
			found := findUniqueLines(out, oldLines)
			if found < 0 {
				return nil, fmt.Errorf("fs.ApplyPatch: hunk for %s does not match", path)
			}
			idx = found
		}
		out = slices.Replace(out, idx, idx+len(oldLines), newLines...)
		delta += len(newLines) - len(oldLines)
	}
	return out, nil
}

func hunkLines(h patchHunk) (oldLines, newLines []string) {
	for _, line := range h.lines {
		switch line.kind {
		case ' ':
			oldLines = append(oldLines, line.text)
			newLines = append(newLines, line.text)
		case '-':
			oldLines = append(oldLines, line.text)
		case '+':
			newLines = append(newLines, line.text)
		}
	}
	return oldLines, newLines
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func findUniqueLines(lines, needle []string) int {
	if len(needle) == 0 {
		return 0
	}
	found := -1
	for i := 0; i+len(needle) <= len(lines); i++ {
		if !equalLines(lines[i:i+len(needle)], needle) {
			continue
		}
		if found >= 0 {
			return -1
		}
		found = i
	}
	return found
}

func parseUnifiedPatch(patch string) (unifiedPatch, error) {
	if strings.TrimSpace(patch) == "" {
		return unifiedPatch{}, errors.New("fs.ApplyPatch: patch must not be empty")
	}
	lines := strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n")
	var parsed unifiedPatch
	var current *filePatch
	var hunk *patchHunk
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "diff --git "):
			hunk = nil
		case strings.HasPrefix(line, "--- "):
			parsed.files = append(parsed.files, filePatch{oldPath: cleanPatchPath(strings.TrimSpace(strings.TrimPrefix(line, "--- ")))})
			current = &parsed.files[len(parsed.files)-1]
			hunk = nil
		case strings.HasPrefix(line, "+++ "):
			if current == nil {
				return unifiedPatch{}, fmt.Errorf("fs.ApplyPatch: +++ header before --- at line %d", i+1)
			}
			current.newPath = cleanPatchPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")))
		case strings.HasPrefix(line, "@@ "):
			if current == nil || current.newPath == "" {
				return unifiedPatch{}, fmt.Errorf("fs.ApplyPatch: hunk before file header at line %d", i+1)
			}
			parsedHunk, err := parseHunkHeader(line)
			if err != nil {
				return unifiedPatch{}, fmt.Errorf("fs.ApplyPatch: line %d: %w", i+1, err)
			}
			current.hunks = append(current.hunks, parsedHunk)
			hunk = &current.hunks[len(current.hunks)-1]
		case strings.HasPrefix(line, `\ No newline at end of file`):
			if hunk == nil || len(hunk.lines) == 0 {
				return unifiedPatch{}, fmt.Errorf("fs.ApplyPatch: misplaced no-newline marker at line %d", i+1)
			}
			last := &hunk.lines[len(hunk.lines)-1]
			last.text = strings.TrimSuffix(last.text, "\n")
		default:
			if hunk == nil {
				continue
			}
			if line == "" && i == len(lines)-1 {
				continue
			}
			if line == "" {
				return unifiedPatch{}, fmt.Errorf("fs.ApplyPatch: empty patch line inside hunk at line %d", i+1)
			}
			kind := line[0]
			if kind != ' ' && kind != '-' && kind != '+' {
				return unifiedPatch{}, fmt.Errorf("fs.ApplyPatch: invalid hunk line at line %d", i+1)
			}
			hunk.lines = append(hunk.lines, patchLine{kind: kind, text: line[1:] + "\n"})
		}
	}
	if len(parsed.files) == 0 {
		return unifiedPatch{}, errors.New("fs.ApplyPatch: no file patches found")
	}
	for _, file := range parsed.files {
		for _, hunk := range file.hunks {
			oldLines, newLines := hunkLines(hunk)
			if len(oldLines) != hunk.oldCount || len(newLines) != hunk.newCount {
				return unifiedPatch{}, fmt.Errorf("fs.ApplyPatch: hunk line count mismatch in %s", file.path())
			}
		}
	}
	return parsed, nil
}

func parseHunkHeader(line string) (patchHunk, error) {
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "@@" {
		return patchHunk{}, fmt.Errorf("invalid hunk header %q", line)
	}
	oldStart, oldCount, err := parseRange(fields[1], '-')
	if err != nil {
		return patchHunk{}, err
	}
	newStart, newCount, err := parseRange(fields[2], '+')
	if err != nil {
		return patchHunk{}, err
	}
	return patchHunk{
		oldStart: oldStart,
		oldCount: oldCount,
		newStart: newStart,
		newCount: newCount,
	}, nil
}

func parseRange(s string, prefix byte) (start, count int, err error) {
	if s == "" || s[0] != prefix {
		return 0, 0, fmt.Errorf("invalid range %q", s)
	}
	body := s[1:]
	startText, countText, found := strings.Cut(body, ",")
	start, err = strconv.Atoi(startText)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range %q", s)
	}
	if !found {
		return start, 1, nil
	}
	count, err = strconv.Atoi(countText)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range %q", s)
	}
	return start, count, nil
}

func cleanPatchPath(path string) string {
	if path == "/dev/null" {
		return path
	}
	if before, _, ok := strings.Cut(path, "\t"); ok {
		path = before
	}
	path = strings.Trim(path, "\"")
	if path == "a" || path == "b" {
		return path
	}
	if rest, ok := strings.CutPrefix(path, "a/"); ok {
		return filepath.Clean(rest)
	}
	if rest, ok := strings.CutPrefix(path, "b/"); ok {
		return filepath.Clean(rest)
	}
	return filepath.Clean(path)
}

func splitTextLines(text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.SplitAfter(text, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func joinTextLines(lines []string) string {
	return strings.Join(lines, "")
}

func sortedUnique(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}
