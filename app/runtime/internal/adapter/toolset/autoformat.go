package toolset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/tools"
)

func withAutoFormat(inner tools.Tool, workdir string) tools.Tool {
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		paths := resolvedMutationPaths(inner, arguments, workdir)
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
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return runFormatter(ctx, "gofmt", "-w", path)
	case ".json":
		if err := formatJSON(path, info.Mode().Perm()); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return nil
	case ".js", ".jsx", ".ts", ".tsx", ".css", ".scss", ".html", ".md", ".yaml", ".yml":
		if _, err := exec.LookPath("prettier"); err != nil {
			return nil
		}
		return runFormatter(ctx, "prettier", "--write", path)
	default:
		return nil
	}
}

func runFormatter(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	target := name
	if len(args) > 0 {
		target = args[len(args)-1]
	}
	if msg == "" {
		return fmt.Errorf("%s: run %s: %w", target, name, err)
	}
	return fmt.Errorf("%s: %s: %w", target, msg, err)
}

func formatJSON(path string, mode os.FileMode) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !json.Valid(data) {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return nil
	}
	buf.WriteByte('\n')
	return writeFormattedFile(path, buf.Bytes(), mode)
}

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
