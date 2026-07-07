package git

import (
	"strconv"
	"strings"
)

// parseUnifiedDiff parses a `git diff` unified patch into per-file DiffFiles.
// Path comes from the +++ (new) / --- (old, for deletes) headers — one path per
// line, so unambiguous even with spaces; status from the extended headers
// (new file / deleted file / rename); added/removed counted from the rows.
func parseUnifiedDiff(patch string) []DiffFile {
	var files []DiffFile
	var cur *DiffFile
	var leftLine, rightLine int

	for line := range strings.SplitSeq(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			files = append(files, DiffFile{Status: StatusModified})
			cur = &files[len(files)-1]
		case cur == nil:
			continue
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = StatusAdded
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = StatusDeleted
		case strings.HasPrefix(line, "rename from "):
			cur.PreviousPath = strings.TrimPrefix(line, "rename from ")
			cur.Status = StatusRenamed
		case strings.HasPrefix(line, "rename to "):
			cur.Path = strings.TrimPrefix(line, "rename to ")
			cur.Status = StatusRenamed
		case strings.HasPrefix(line, "Binary files "):
			cur.Binary = true
		case strings.HasPrefix(line, "--- "):
			if p := strings.TrimPrefix(line, "--- "); cur.Path == "" && p != "/dev/null" {
				cur.Path = strings.TrimPrefix(p, "a/")
			}
		case strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p != "/dev/null" {
				cur.Path = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "@@"):
			leftLine, rightLine = parseHunkHeader(line)
			cur.Rows = append(cur.Rows, Row{Type: "hunk", Text: line})
		case strings.HasPrefix(line, "+"):
			cur.Rows = append(cur.Rows, Row{Type: "added", RightLine: rightLine, Code: line[1:]})
			rightLine++
			cur.Added++
		case strings.HasPrefix(line, "-"):
			cur.Rows = append(cur.Rows, Row{Type: "deleted", LeftLine: leftLine, Code: line[1:]})
			leftLine++
			cur.Removed++
		case strings.HasPrefix(line, " "):
			cur.Rows = append(cur.Rows, Row{Type: "context", LeftLine: leftLine, RightLine: rightLine, Code: line[1:]})
			leftLine++
			rightLine++
		}
	}
	return files
}

// parseHunkHeader pulls the left/right start lines out of "@@ -L,S +R,S @@ …".
func parseHunkHeader(h string) (left, right int) {
	for f := range strings.FieldsSeq(h) {
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case '-':
			left = atoiBeforeComma(f[1:])
		case '+':
			right = atoiBeforeComma(f[1:])
		}
	}
	return left, right
}

func atoiBeforeComma(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}
