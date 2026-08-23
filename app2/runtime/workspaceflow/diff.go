package workspaceflow

import (
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func parsePatch(patch string, limit int) ([]protocol.FileDiff, bool) {
	if limit <= 0 {
		limit = 5000
	}
	files := make([]protocol.FileDiff, 0)
	var current *protocol.FileDiff
	left, right, rows := 0, 0, 0
	truncated := false
	flush := func() bool {
		if current == nil {
			return true
		}
		if rows+len(current.Rows) > limit {
			truncated = true
			return false
		}
		rows += len(current.Rows)
		files = append(files, *current)
		return true
	}
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			if !flush() {
				break
			}
			path := diffHeaderPath(strings.TrimPrefix(line, "diff --git "))
			current = &protocol.FileDiff{
				Path: path, Status: protocol.FileStatusModified, Rows: []protocol.DiffRow{},
			}
			left, right = 0, 0
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "new file"):
			current.Status = protocol.FileStatusAdded
		case strings.HasPrefix(line, "deleted file"):
			current.Status = protocol.FileStatusDeleted
		case strings.HasPrefix(line, "rename from "):
			current.Status = protocol.FileStatusRenamed
			current.PreviousPath = parsePatchPath(strings.TrimPrefix(line, "rename from "), "")
		case strings.HasPrefix(line, "rename to "):
			current.Status = protocol.FileStatusRenamed
			current.Path = parsePatchPath(strings.TrimPrefix(line, "rename to "), "")
		case strings.HasPrefix(line, "Binary files"):
			current.Binary = true
			if path := binaryPatchPath(line); path != "" {
				current.Path = path
			}
		case strings.HasPrefix(line, "--- "):
			path := parsePatchPath(strings.TrimPrefix(line, "--- "), "a/")
			if current.Path == "" && path != "/dev/null" {
				current.Path = path
			}
		case strings.HasPrefix(line, "+++ "):
			path := parsePatchPath(strings.TrimPrefix(line, "+++ "), "b/")
			if path != "/dev/null" {
				current.Path = path
			}
		case strings.HasPrefix(line, "@@"):
			left, right = parseHunk(line)
			current.Rows = append(current.Rows, protocol.DiffRow{
				Type: protocol.DiffRowHunk,
				Text: line,
			})
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			current.Rows = append(current.Rows, protocol.DiffRow{
				Type: protocol.DiffRowAdded,
				RightLine: right,
				Code: line[1:],
			})
			right++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			current.Rows = append(current.Rows, protocol.DiffRow{
				Type: protocol.DiffRowDeleted,
				LeftLine: left,
				Code: line[1:],
			})
			left++
		case strings.HasPrefix(line, " "):
			current.Rows = append(current.Rows, protocol.DiffRow{
				Type: protocol.DiffRowContext,
				LeftLine: left,
				RightLine: right,
				Code: line[1:],
			})
			left++
			right++
		}
	}
	if !truncated {
		flush()
	}
	for index := range files {
		added, removed := 0, 0
		for _, row := range files[index].Rows {
			if row.Type == protocol.DiffRowAdded {
				added++
			}
			if row.Type == protocol.DiffRowDeleted {
				removed++
			}
		}
		if !files[index].Binary {
			files[index].Added, files[index].Removed = &added, &removed
		}
	}
	return files, truncated
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
		if ok {
			return strings.TrimPrefix(path, "b/")
		}
		return ""
	}
	_, path, found := strings.Cut(value, " b/")
	if !found {
		return ""
	}
	return path
}

func cutQuotedPath(value string) (string, string, bool) {
	for index := 1; index < len(value); index++ {
		if value[index] != '\"' {
			continue
		}
		backslashes := 0
		for cursor := index - 1; cursor >= 0 && value[cursor] == '\\'; cursor-- {
			backslashes++
		}
		if backslashes%2 != 0 {
			continue
		}
		quoted := value[:index+1]
		unquoted, err := strconv.Unquote(quoted)
		return unquoted, value[index+1:], err == nil
	}
	return "", value, false
}

func binaryPatchPath(line string) string {
	marker := " and b/"
	index := strings.LastIndex(line, marker)
	if index < 0 {
		return ""
	}
	return strings.TrimSuffix(line[index+len(marker):], " differ")
}

func parseHunk(line string) (int, int) {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return 0, 0
	}
	parse := func(value string) int {
		value = strings.TrimLeft(value, "+-")
		value, _, _ = strings.Cut(value, ",")
		parsed, _ := strconv.Atoi(value)
		return parsed
	}
	return parse(parts[1]), parse(parts[2])
}
