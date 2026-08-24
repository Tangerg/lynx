package toolset

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/tools/fs"
)

const (
	maxRuntimeReadFileBytes   int64 = 8 << 20
	maxRuntimeReadOutputBytes       = 1 << 20
	maxRuntimeReadLineBytes         = 1 << 20
)

var errRuntimeReadFileTooLarge = errors.New("toolset: read file exceeds the 8 MiB limit")

// runtimeReadExecutor overrides only the model-facing Read operation. The
// embedded executor continues to own every other fs operation, while Runtime
// owns the complete-file and model-result envelopes its read consumer needs.
type runtimeReadExecutor struct {
	fs.Executor
	root string
}

func newRuntimeReadTool(root string, executor fs.Executor) *fs.ReadTool {
	if executor == nil {
		executor = fs.NewLocalExecutor(root)
	}
	return fs.NewReadTool(runtimeReadExecutor{Executor: executor, root: root})
}

func (executor runtimeReadExecutor) Read(ctx context.Context, input fs.ReadInput) (_ fs.ReadOutput, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return fs.ReadOutput{}, cause
	}
	path, err := runtimeReadPath(executor.root, input.Path)
	if err != nil {
		return fs.ReadOutput{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return fs.ReadOutput{}, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	info, err := file.Stat()
	if err != nil {
		return fs.ReadOutput{}, err
	}
	if !info.Mode().IsRegular() {
		return fs.ReadOutput{}, fmt.Errorf("toolset: read %s: unsupported file mode %s", input.Path, info.Mode().Type())
	}
	if info.Size() > maxRuntimeReadFileBytes {
		return fs.ReadOutput{}, fmt.Errorf("%w: %s uses %d bytes", errRuntimeReadFileTooLarge, input.Path, info.Size())
	}
	return scanRuntimeRead(ctx, file, input)
}

func scanRuntimeRead(ctx context.Context, file io.Reader, input fs.ReadInput) (fs.ReadOutput, error) {
	counter := &runtimeReadCounter{reader: runtimeReadContextReader{
		ctx:    ctx,
		reader: io.LimitReader(file, maxRuntimeReadFileBytes+1),
	}}
	reader := bufio.NewReaderSize(counter, 64<<10)

	start := max(input.Offset, 0)
	var content strings.Builder
	selected := 0
	total := 0
	outputTruncated := false
	consume := func(line []byte) error {
		if total == 0 {
			line = stripUTF8BOM(line)
		}
		if len(line) > maxRuntimeReadLineBytes {
			return fmt.Errorf("toolset: read %s: line %d exceeds the 1 MiB limit", input.Path, total+1)
		}
		if !utf8.Valid(line) || bytes.IndexByte(line, 0) >= 0 {
			return fs.ErrBinaryFile
		}
		index := total
		total++
		if index < start || outputTruncated || (input.Limit > 0 && selected >= input.Limit) {
			return nil
		}
		separator := 0
		if selected > 0 {
			separator = 1
		}
		if content.Len()+separator+len(line) > maxRuntimeReadOutputBytes {
			outputTruncated = true
			return nil
		}
		if separator > 0 {
			content.WriteByte('\n')
		}
		content.Write(line)
		selected++
		return nil
	}

	line := make([]byte, 0, 64<<10)
	endedWithNewline := false
scan:
	for {
		fragment, readErr := reader.ReadSlice('\n')
		rawLineLimit := maxRuntimeReadLineBytes + 2
		if total == 0 {
			rawLineLimit += 3 // an optional UTF-8 BOM is not part of line content
		}
		if len(line)+len(fragment) > rawLineLimit {
			return fs.ReadOutput{}, fmt.Errorf("toolset: read %s: line %d exceeds the 1 MiB limit", input.Path, total+1)
		}
		line = append(line, fragment...)
		switch {
		case readErr == nil:
			line = line[:len(line)-1]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if err := consume(line); err != nil {
				return fs.ReadOutput{}, err
			}
			line = line[:0]
			endedWithNewline = true
		case errors.Is(readErr, bufio.ErrBufferFull):
			endedWithNewline = false
			continue
		case errors.Is(readErr, io.EOF):
			if counter.bytes > maxRuntimeReadFileBytes {
				return fs.ReadOutput{}, fmt.Errorf("%w: %s grew while reading", errRuntimeReadFileTooLarge, input.Path)
			}
			if len(line) > 0 {
				if err := consume(line); err != nil {
					return fs.ReadOutput{}, err
				}
			} else if total == 0 || endedWithNewline {
				if err := consume(nil); err != nil {
					return fs.ReadOutput{}, err
				}
			}
			break scan
		default:
			return fs.ReadOutput{}, fmt.Errorf("toolset: scan %s: %w", input.Path, readErr)
		}
	}
	if cause := context.Cause(ctx); cause != nil {
		return fs.ReadOutput{}, cause
	}

	start = min(start, total)
	end := start + selected
	return fs.ReadOutput{
		Content: content.String(), StartLine: start, EndLine: end, TotalLines: total,
		Truncated: start > 0 || end < total || outputTruncated,
	}, nil
}

func runtimeReadPath(root, path string) (string, error) {
	if path == "" {
		return "", fs.ErrEmptyPath
	}
	if root == "" || filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Join(root, path), nil
}

func stripUTF8BOM(line []byte) []byte {
	if len(line) >= 3 && line[0] == 0xef && line[1] == 0xbb && line[2] == 0xbf {
		return line[3:]
	}
	return line
}

type runtimeReadContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader runtimeReadContextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(reader.ctx); cause != nil {
		return 0, cause
	}
	read, err := reader.reader.Read(buffer)
	if cause := context.Cause(reader.ctx); cause != nil {
		return read, cause
	}
	return read, err
}

type runtimeReadCounter struct {
	reader io.Reader
	bytes  int64
}

func (reader *runtimeReadCounter) Read(buffer []byte) (int, error) {
	read, err := reader.reader.Read(buffer)
	if read > 0 {
		reader.bytes += int64(read)
	}
	return read, err
}
