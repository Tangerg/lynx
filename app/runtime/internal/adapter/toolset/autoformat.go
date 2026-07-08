package toolset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

func withAutoFormat(inner chat.Tool, workdir string) chat.Tool {
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		paths := resolvedMutatedPaths(inner, arguments, workdir)
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
	if err != nil || info.IsDir() {
		return nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return runFormatter(ctx, "gofmt", "-w", path)
	case ".json":
		return formatJSON(path, info.Mode().Perm())
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
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("%s: %s", args[len(args)-1], msg)
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
	return os.WriteFile(path, buf.Bytes(), mode)
}
