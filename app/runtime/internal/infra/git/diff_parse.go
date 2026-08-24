package git

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// parseUnifiedDiff parses a byte-bounded `git diff` patch into whole-file
// DiffFiles. It stops before entering a file that cannot fit the file or row
// budget, so truncation never retains or returns a partial file. Path comes
// from the +++ (new) / --- (old, for deletes) headers; status comes from the
// extended headers, and added/removed are counted from retained rows.
func parseUnifiedDiff(patch []byte, maxFiles, maxRows int) ([]DiffFile, bool, error) {
	if maxFiles <= 0 || maxRows <= 0 {
		return nil, false, fmt.Errorf("%w: diff projection requires positive limits", ErrResultTooLarge)
	}
	var files []DiffFile
	var cur *DiffFile
	var leftLine, rightLine, rows int
	flush := func() bool {
		if cur == nil {
			return true
		}
		if len(files) >= maxFiles || rows+len(cur.Rows) > maxRows {
			return false
		}
		rows += len(cur.Rows)
		files = append(files, *cur)
		return true
	}

	for encoded := range bytes.SplitSeq(patch, []byte{'\n'}) {
		switch {
		case bytes.HasPrefix(encoded, []byte("diff --git ")):
			if !flush() {
				return files, true, nil
			}
			if len(files) >= maxFiles {
				return files, true, nil
			}
			cur = &DiffFile{
				Path:   diffHeaderPath(strings.TrimPrefix(string(encoded), "diff --git ")),
				Status: StatusModified,
			}
			leftLine, rightLine = 0, 0
		case cur == nil:
			continue
		case bytes.HasPrefix(encoded, []byte("new file mode")):
			cur.Status = StatusAdded
		case bytes.HasPrefix(encoded, []byte("deleted file mode")):
			cur.Status = StatusDeleted
		case bytes.HasPrefix(encoded, []byte("rename from ")):
			cur.PreviousPath = parsePatchPath(strings.TrimPrefix(string(encoded), "rename from "), "")
			cur.Status = StatusRenamed
		case bytes.HasPrefix(encoded, []byte("rename to ")):
			cur.Path = parsePatchPath(strings.TrimPrefix(string(encoded), "rename to "), "")
			cur.Status = StatusRenamed
		case bytes.HasPrefix(encoded, []byte("Binary files ")):
			cur.Binary = true
			if path := binaryPatchPath(string(encoded)); path != "" {
				cur.Path = path
			}
		case bytes.HasPrefix(encoded, []byte("--- ")):
			if path := parsePatchPath(strings.TrimPrefix(string(encoded), "--- "), "a/"); cur.Path == "" && path != "/dev/null" {
				cur.Path = path
			}
		case bytes.HasPrefix(encoded, []byte("+++ ")):
			if path := parsePatchPath(strings.TrimPrefix(string(encoded), "+++ "), "b/"); path != "/dev/null" {
				cur.Path = path
			}
		case bytes.HasPrefix(encoded, []byte("@@")):
			if rows+len(cur.Rows) >= maxRows {
				return files, true, nil
			}
			var err error
			line := string(encoded)
			leftLine, rightLine, err = parseHunkHeader(line)
			if err != nil {
				return nil, false, err
			}
			cur.Rows = append(cur.Rows, Row{Type: RowHunk, Text: line})
		case len(encoded) > 0 && encoded[0] == '+':
			if rows+len(cur.Rows) >= maxRows {
				return files, true, nil
			}
			cur.Rows = append(cur.Rows, Row{Type: RowAdded, RightLine: rightLine, Code: string(encoded[1:])})
			rightLine++
			cur.Added++
		case len(encoded) > 0 && encoded[0] == '-':
			if rows+len(cur.Rows) >= maxRows {
				return files, true, nil
			}
			cur.Rows = append(cur.Rows, Row{Type: RowDeleted, LeftLine: leftLine, Code: string(encoded[1:])})
			leftLine++
			cur.Removed++
		case len(encoded) > 0 && encoded[0] == ' ':
			if rows+len(cur.Rows) >= maxRows {
				return files, true, nil
			}
			cur.Rows = append(cur.Rows, Row{Type: RowContext, LeftLine: leftLine, RightLine: rightLine, Code: string(encoded[1:])})
			leftLine++
			rightLine++
		}
	}
	if !flush() {
		return files, true, nil
	}
	return files, false, nil
}

func parsePatchPath(value, prefix string) string {
	if strings.HasPrefix(value, "\"") {
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
	}
	return strings.TrimPrefix(value, prefix)
}

func diffHeaderPath(value string) string {
	if strings.HasPrefix(value, "\"") {
		_, remainder, ok := cutQuotedPath(value)
		if !ok {
			return ""
		}
		path, _, ok := cutQuotedPath(strings.TrimSpace(remainder))
		if !ok {
			return ""
		}
		return strings.TrimPrefix(path, "b/")
	}
	_, path, found := strings.Cut(value, " b/")
	if !found {
		return ""
	}
	return path
}

func cutQuotedPath(value string) (string, string, bool) {
	for index := 1; index < len(value); index++ {
		if value[index] != '"' {
			continue
		}
		backslashes := 0
		for cursor := index - 1; cursor >= 0 && value[cursor] == '\\'; cursor-- {
			backslashes++
		}
		if backslashes%2 != 0 {
			continue
		}
		unquoted, err := strconv.Unquote(value[:index+1])
		return unquoted, value[index+1:], err == nil
	}
	return "", value, false
}

func binaryPatchPath(line string) string {
	const marker = " and b/"
	index := strings.LastIndex(line, marker)
	if index < 0 {
		return ""
	}
	return strings.TrimSuffix(line[index+len(marker):], " differ")
}

// parseHunkHeader pulls the left/right start lines out of "@@ -L,S +R,S @@ …".
func parseHunkHeader(h string) (left, right int, err error) {
	fields := strings.Fields(h)
	if len(fields) < 4 || fields[0] != "@@" || fields[3] != "@@" ||
		len(fields[1]) < 2 || fields[1][0] != '-' ||
		len(fields[2]) < 2 || fields[2][0] != '+' {
		return 0, 0, fmt.Errorf("git: malformed hunk header %q", h)
	}
	left, err = atoiBeforeComma(fields[1][1:])
	if err != nil {
		return 0, 0, fmt.Errorf("git: malformed hunk header %q: %w", h, err)
	}
	right, err = atoiBeforeComma(fields[2][1:])
	if err != nil {
		return 0, 0, fmt.Errorf("git: malformed hunk header %q: %w", h, err)
	}
	return left, right, nil
}

func atoiBeforeComma(s string) (int, error) {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid line number %q", s)
	}
	return n, nil
}
