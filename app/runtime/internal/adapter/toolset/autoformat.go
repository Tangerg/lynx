package toolset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

const (
	maxAutoFormatFileBytes       int64 = 8 << 20
	maxAutoFormatDiagnosticBytes       = 64 << 10
	autoFormatProcessWaitDelay         = time.Second
)

var errAutoFormatFileTooLarge = errors.New("auto-format: file exceeds the 8 MiB limit")

func withAutoFormat(inner toolcontract.Tool, cwd string) toolcontract.Tool {
	return decorateCall(inner, func(ctx context.Context, arguments string) (string, error) {
		paths, err := resolvedMutationPaths(inner, arguments, cwd)
		if err != nil {
			return "", fmt.Errorf("inspect mutation paths before formatting: %w", err)
		}
		out, err := inner.Call(ctx, arguments)
		if err != nil || len(paths) == 0 {
			return out, err
		}
		var failed []string
		for _, path := range paths {
			if formatErr := formatPath(ctx, path); formatErr != nil {
				failed = append(failed, formatErr.Error())
			}
		}
		if len(failed) == 0 {
			return out, nil
		}
		return out + "\n\nAuto-format skipped or failed:\n- " + strings.Join(failed, "\n- "), nil
	})
}

func formatPath(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: inspect before formatting: %w", path, err)
	}
	if info.IsDir() {
		return nil
	}
	extension := strings.ToLower(filepath.Ext(path))
	prettier := ""
	switch extension {
	case ".go", ".json":
	case ".js", ".jsx", ".ts", ".tsx", ".css", ".scss", ".html", ".md", ".yaml", ".yml":
		prettier, err = exec.LookPath("prettier")
		if err != nil {
			return nil
		}
	default:
		return nil
	}

	input, err := readAutoFormatFile(ctx, path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var formatted []byte
	switch extension {
	case ".go":
		formatted, err = format.Source(input)
		if err != nil {
			return fmt.Errorf("%s: gofmt: %w", path, err)
		}
	case ".json":
		if !json.Valid(input) {
			return nil
		}
		var buffer bytes.Buffer
		if indentErr := json.Indent(&buffer, input, "", "  "); indentErr != nil {
			return nil
		}
		buffer.WriteByte('\n')
		formatted = buffer.Bytes()
	default:
		formatted, err = runFormatter(ctx, input, prettier, "--stdin-filepath", path)
		if err != nil {
			return err
		}
	}
	if len(formatted) > int(maxAutoFormatFileBytes) {
		return fmt.Errorf("%s: %w: formatted output uses %d bytes", path, errAutoFormatFileTooLarge, len(formatted))
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if bytes.Equal(input, formatted) {
		return nil
	}
	return writeFormattedFile(path, formatted, info.Mode().Perm())
}

func runFormatter(ctx context.Context, input []byte, name string, args ...string) ([]byte, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	stdout := &formatOutputBuffer{limit: int(maxAutoFormatFileBytes)}
	stderr := &formatOutputBuffer{limit: maxAutoFormatDiagnosticBytes}
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = autoFormatProcessWaitDelay
	runErr := cmd.Run()
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	target := name
	if len(args) > 0 {
		target = args[len(args)-1]
	}
	if stdout.overflow {
		return nil, fmt.Errorf("%s: %s output exceeds 8 MiB", target, name)
	}
	if runErr == nil && !stderr.overflow {
		return bytes.Clone(stdout.Bytes()), nil
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		if runErr == nil {
			return nil, fmt.Errorf("%s: %s diagnostic output was truncated", target, name)
		}
		return nil, fmt.Errorf("%s: run %s: %w", target, name, runErr)
	}
	if runErr == nil {
		return nil, fmt.Errorf("%s: %s", target, msg)
	}
	return nil, fmt.Errorf("%s: %s: %w", target, msg, runErr)
}

func readAutoFormatFile(ctx context.Context, path string) (_ []byte, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxAutoFormatFileBytes {
		return nil, fmt.Errorf("%w: file uses %d bytes", errAutoFormatFileTooLarge, info.Size())
	}
	content, err := io.ReadAll(io.LimitReader(
		autoFormatContextReader{ctx: ctx, reader: file},
		maxAutoFormatFileBytes+1,
	))
	if err != nil {
		return nil, err
	}
	if len(content) > int(maxAutoFormatFileBytes) {
		return nil, fmt.Errorf("%w: file grew while reading", errAutoFormatFileTooLarge)
	}
	return content, nil
}

type autoFormatContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (a autoFormatContextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(a.ctx); cause != nil {
		return 0, cause
	}
	read, err := a.reader.Read(buffer)
	if cause := context.Cause(a.ctx); cause != nil {
		return read, cause
	}
	return read, err
}

type formatOutputBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (f *formatOutputBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := f.limit - f.buffer.Len()
	if remaining > 0 {
		_, _ = f.buffer.Write(value[:min(len(value), remaining)])
	}
	if len(value) > remaining {
		f.overflow = true
	}
	return written, nil
}

func (f *formatOutputBuffer) String() string {
	if f.overflow {
		return f.buffer.String() + "\n... [formatter diagnostic truncated] ..."
	}
	return f.buffer.String()
}

func (f *formatOutputBuffer) Bytes() []byte { return f.buffer.Bytes() }

func writeFormattedFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".format-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
