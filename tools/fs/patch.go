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

type unifiedPatch struct {
	files []filePatch
}

// paths is every file this patch touches, INCLUDING a move's origin. Callers use
// it to lock and to gate — the app wraps this tool in a guard stack that reads
// these paths to serialize writes, refuse protected directories, and require a
// prior read — so a move that reported only its destination would leave the file
// it removes outside all three.
func (p unifiedPatch) paths() []string {
	paths := make([]string, 0, len(p.files))
	for _, file := range p.files {
		paths = append(paths, file.touches()...)
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

// duplicatePath reports a path two file patches both touch. Endpoints count, not
// just destinations: patching a file and moving another one onto it are two edits
// to one path, and applying both would make the result depend on their order.
func (p unifiedPatch) duplicatePath() string {
	seen := make(map[string]struct{}, len(p.files))
	for _, file := range p.files {
		for _, path := range file.touches() {
			if _, ok := seen[path]; ok {
				return path
			}
			seen[path] = struct{}{}
		}
	}
	return ""
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

// moved reports the fourth shape: both headers name a real file and they differ,
// so the content is read at oldPath, patched, and lands at newPath while oldPath
// goes away. It is the one shape whose two endpoints are different files.
func (p filePatch) moved() bool {
	return p.oldPath != "" && p.newPath != "" &&
		p.oldPath != "/dev/null" && p.newPath != "/dev/null" &&
		p.oldPath != p.newPath
}

// touches is every path this file patch reads, writes or removes.
func (p filePatch) touches() []string {
	if p.moved() {
		return []string{p.oldPath, p.newPath}
	}
	return []string{p.path()}
}

func (p filePatch) validate() error {
	if p.oldPath == "" || p.newPath == "" {
		return errors.New("fs.ApplyPatch: file patch is missing ---/+++ headers")
	}
	// A pure rename is the one patch with nothing to apply: git emits it with two
	// headers and no hunks, and there is no content change to describe. Every other
	// shape without a hunk says nothing at all.
	if len(p.hunks) == 0 && !p.moved() {
		return errors.New("fs.ApplyPatch: file patch has no hunks")
	}
	if p.oldPath != "/dev/null" {
		if err := validatePatchPath(p.oldPath); err != nil {
			return err
		}
	}
	if p.newPath != "/dev/null" {
		if err := validatePatchPath(p.newPath); err != nil {
			return err
		}
	}
	return nil
}

func (p filePatch) apply(lines []string) ([]string, error) {
	out := slices.Clone(lines)
	delta := 0
	for _, hunk := range p.hunks {
		oldLines, newLines := hunk.splitLines()
		idx := hunk.oldStart - 1 + delta
		if hunk.oldStart == 0 {
			idx = delta
		}
		if idx < 0 || idx+len(oldLines) > len(out) || !equalLines(out[idx:idx+len(oldLines)], oldLines) {
			found := findUniqueLines(out, oldLines)
			if found < 0 {
				return nil, fmt.Errorf("fs.ApplyPatch: hunk for %s does not match", p.path())
			}
			idx = found
		}
		out = slices.Replace(out, idx, idx+len(oldLines), newLines...)
		delta += len(newLines) - len(oldLines)
	}
	return out, nil
}

type patchHunk struct {
	oldStart int
	oldCount int
	newStart int
	newCount int
	lines    []patchLine
}

func (h patchHunk) splitLines() (oldLines, newLines []string) {
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

type patchLine struct {
	kind byte
	text string
}

func patchPaths(patch string) ([]string, error) {
	parsed, err := parseUnifiedPatch(patch)
	if err != nil {
		return nil, err
	}
	return parsed.paths(), nil
}

func (l *LocalExecutor) ApplyPatch(_ context.Context, in ApplyPatchInput) (ApplyPatchOutput, error) {
	parsed, err := parseUnifiedPatch(in.Patch)
	if err != nil {
		return ApplyPatchOutput{}, err
	}
	if path := parsed.duplicatePath(); path != "" {
		return ApplyPatchOutput{}, fmt.Errorf("fs.ApplyPatch: duplicate file patch for %s", path)
	}

	resolved := make([]patchTarget, len(parsed.files))
	var locks []string
	for i, file := range parsed.files {
		if err := file.validate(); err != nil {
			return ApplyPatchOutput{}, err
		}
		target, err := l.resolveTarget(file)
		if err != nil {
			return ApplyPatchOutput{}, err
		}
		resolved[i] = target
		locks = append(locks, target.locks()...)
	}

	// Both endpoints of a move are locked: it removes one file and creates
	// another, and holding only the destination would let a concurrent write to the
	// origin land in a file this call is about to delete.
	for _, path := range sortedUnique(locks) {
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

func validatePatchPath(path string) error {
	if path == "" || path == "." || path == string(filepath.Separator) {
		return fmt.Errorf("fs.ApplyPatch: invalid file path %q", path)
	}
	return nil
}

// patchTarget is one file patch's resolved endpoints: where its content is read
// and where it lands. They are the same file for every shape but a move, and one
// of them is empty when the patch creates or deletes.
type patchTarget struct {
	from string
	to   string
}

func (t patchTarget) locks() []string {
	if t.from != "" && t.to != "" && t.from != t.to {
		return []string{t.from, t.to}
	}
	if t.to != "" {
		return []string{t.to}
	}
	return []string{t.from}
}

func (l *LocalExecutor) resolveTarget(file filePatch) (patchTarget, error) {
	var target patchTarget
	if file.oldPath != "/dev/null" {
		from, err := l.resolve(file.oldPath)
		if err != nil {
			return patchTarget{}, err
		}
		target.from = from
	}
	if file.newPath != "/dev/null" {
		to, err := l.resolve(file.newPath)
		if err != nil {
			return patchTarget{}, err
		}
		target.to = to
	}
	return target, nil
}

// preparedPatch is one file patch's committed outcome, computed before anything
// is written so a patch that cannot apply changes nothing.
type preparedPatch struct {
	// path is where the content lands, empty for a delete.
	path string
	// source is the file to remove once the content has landed: a delete's own
	// path, or the origin of a move. Empty when nothing is removed.
	source string
	data   []byte
	mode   os.FileMode
	result PatchFileOutput
}

// commit writes before it removes, so a failure between the two leaves the
// content somewhere rather than nowhere.
func (p preparedPatch) commit() error {
	if p.path != "" {
		if err := atomicWriteFile(p.path, p.data, p.mode); err != nil {
			return err
		}
	}
	if p.source != "" && p.source != p.path {
		return os.Remove(p.source)
	}
	return nil
}

func (l *LocalExecutor) preparePatch(file filePatch, target patchTarget) (preparedPatch, error) {
	// A patch may not land on a file it did not open. Create says so by having no
	// origin; a move has one, but its destination is a new file all the same — and
	// without this check a mistaken destination would silently overwrite whatever
	// was there, which is the one outcome a rename must never produce.
	if file.created() || file.moved() {
		if _, err := os.Stat(target.to); err == nil {
			return preparedPatch{}, fmt.Errorf("fs.ApplyPatch: %s: file already exists", file.newPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return preparedPatch{}, fmt.Errorf("fs.ApplyPatch: %s: %w", file.newPath, err)
		}
	}

	mode := os.FileMode(0o644)
	var lines []string
	hadBOM, hadCRLF := false, false
	if !file.created() {
		info, err := os.Stat(target.from)
		if err != nil {
			return preparedPatch{}, err
		}
		mode = info.Mode().Perm()
		data, err := os.ReadFile(target.from)
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

	patched, err := file.apply(lines)
	if err != nil {
		return preparedPatch{}, err
	}
	if file.deleted() {
		if len(patched) != 0 {
			return preparedPatch{}, fmt.Errorf("fs.ApplyPatch: delete %s: patched content is not empty", file.path())
		}
		return preparedPatch{
			source: target.from,
			result: PatchFileOutput{Path: file.path(), Hunks: len(file.hunks), Deleted: true},
		}, nil
	}

	result := PatchFileOutput{
		Path:    file.path(),
		Hunks:   len(file.hunks),
		Created: file.created(),
	}
	prepared := preparedPatch{
		path: target.to,
		data: restoreFormat(joinTextLines(patched), hadBOM, hadCRLF),
		mode: mode,
	}
	if file.moved() {
		// The origin is reported, not just the destination: "moved" without saying
		// from where leaves the model to infer which of its files stopped existing.
		prepared.source = target.from
		result.MovedFrom = file.oldPath
	}
	prepared.result = result
	return prepared, nil
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
			oldLines, newLines := hunk.splitLines()
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
	if start < 0 {
		return 0, 0, fmt.Errorf("invalid range %q", s)
	}
	if !found {
		return start, 1, nil
	}
	count, err = strconv.Atoi(countText)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range %q", s)
	}
	if count < 0 {
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
