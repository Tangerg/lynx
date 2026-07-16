package fs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Read does not lock — concurrent reads are fine and a slightly stale
// read while another goroutine writes is acceptable (atomic-rename in
// Write means the caller sees either the old file in full or the new file in
// full, never a torn write).
func (l *LocalExecutor) Read(_ context.Context, in ReadInput) (ReadOutput, error) {
	path, err := l.resolve(in.Path)
	if err != nil {
		return ReadOutput{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReadOutput{}, err
	}
	if looksBinary(data) {
		return ReadOutput{}, ErrBinaryFile
	}

	content, _, _ := normalizeText(data)
	lines := strings.Split(content, "\n")
	total := len(lines)

	start := min(max(in.Offset, 0), total)
	end := total
	if in.Limit > 0 {
		end = min(start+in.Limit, total)
	}
	readContent := strings.Join(lines[start:end], "\n")
	byteTruncated := false
	if in.MaxBytes > 0 {
		readContent, byteTruncated = truncateTextBytes(readContent, in.MaxBytes)
	}

	return ReadOutput{
		Content:    readContent,
		StartLine:  start,
		EndLine:    end,
		TotalLines: total,
		Truncated:  end < total || byteTruncated,
	}, nil
}

func (l *LocalExecutor) Write(_ context.Context, in WriteInput) (WriteOutput, error) {
	if strings.ContainsRune(in.Content, 0) {
		return WriteOutput{}, fmt.Errorf("fs.LocalExecutor.Write: %w", ErrBinaryFile)
	}
	path, err := l.resolve(in.Path)
	if err != nil {
		return WriteOutput{}, err
	}

	unlock := l.lockPath(path)
	defer unlock()

	// Detect existing format + permissions so an overwrite preserves
	// CRLF / BOM / mode instead of silently flipping them.
	mode := os.FileMode(0o644)
	hadBOM, hadCRLF := false, false
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		if !in.Append {
			if existing, err := os.ReadFile(path); err == nil {
				hadBOM = hasUTF8BOM(existing)
				hadCRLF = bytes.Contains(existing, []byte("\r\n"))
			}
		}
	}

	if in.Append {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return WriteOutput{}, err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, mode)
		if err != nil {
			return WriteOutput{}, err
		}
		defer file.Close()
		n, err := file.WriteString(in.Content)
		if err != nil {
			return WriteOutput{}, err
		}
		return WriteOutput{BytesWritten: n}, nil
	}

	out := restoreFormat(in.Content, hadBOM, hadCRLF)
	if err := atomicWriteFile(path, out, mode); err != nil {
		return WriteOutput{}, err
	}
	return WriteOutput{BytesWritten: len(out)}, nil
}

func (l *LocalExecutor) Edit(_ context.Context, in EditInput) (EditOutput, error) {
	path, err := l.resolve(in.Path)
	if err != nil {
		return EditOutput{}, err
	}

	unlock := l.lockPath(path)
	defer unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return EditOutput{}, err
	}
	if looksBinary(data) {
		return EditOutput{}, ErrBinaryFile
	}

	content, hadBOM, hadCRLF := normalizeText(data)
	updated, replacements, err := (EditOperation{
		OldString:  in.OldString,
		NewString:  in.NewString,
		ReplaceAll: in.ReplaceAll,
	}).apply(content, in.Path)
	if err != nil {
		return EditOutput{}, err
	}

	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	out := restoreFormat(updated, hadBOM, hadCRLF)
	if err := atomicWriteFile(path, out, mode); err != nil {
		return EditOutput{}, err
	}
	return EditOutput{Replacements: replacements}, nil
}

func (op EditOperation) apply(content, path string) (string, int, error) {
	if op.OldString == "" {
		return "", 0, errors.New("old_string must not be empty")
	}
	occurrences := strings.Count(content, op.OldString)
	switch {
	case occurrences == 0:
		// Exact match failed — fall back to a whitespace-tolerant match so a
		// snippet that drifted on indentation / trailing whitespace still edits,
		// but ONLY when it's unambiguous: a near-match that hits several regions
		// is refused, never guessed (a wrong edit is worse than a clear failure).
		start, end, matches := fuzzyEditRegion(content, op.OldString)
		switch matches {
		case 0:
			return "", 0, fmt.Errorf("old_string not found in %s", path)
		case 1:
			return content[:start] + op.NewString + content[end:], 1, nil
		default:
			return "", 0, fmt.Errorf("old_string not found exactly in %s; %d regions match apart from whitespace — copy it verbatim (or add surrounding lines to disambiguate)", path, matches)
		}
	case occurrences > 1 && !op.ReplaceAll:
		return "", 0, fmt.Errorf("old_string matches %d times in %s — set replace_all=true to confirm", occurrences, path)
	default:
		n := 1
		if op.ReplaceAll {
			n = -1
		}
		replacements := occurrences
		if !op.ReplaceAll {
			replacements = 1
		}
		return strings.Replace(content, op.OldString, op.NewString, n), replacements, nil
	}
}

// binarySniffLen matches git's heuristic — a NUL in the first 8 KiB
// means the file is treated as binary.
const binarySniffLen = 8192

func looksBinary(data []byte) bool {
	sniff := data
	if len(sniff) > binarySniffLen {
		sniff = sniff[:binarySniffLen]
	}
	return bytes.IndexByte(sniff, 0) >= 0
}

func truncateTextBytes(text string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text, false
	}
	end := 0
	for i := range text {
		if i > maxBytes {
			break
		}
		end = i
	}
	return text[:end], true
}
