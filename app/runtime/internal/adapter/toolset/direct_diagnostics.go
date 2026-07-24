package toolset

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/component/pathidentity"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/tools"
	"github.com/Tangerg/lynx/tools/fs"
)

// directTools is the small, read-only capability set valid without an agent
// process. Keep this list explicit: being available to a model does not make a
// tool valid for a client-driven call.
func directTools(root string) []tools.Tool {
	executor := fs.NewLocalExecutor(root)
	return []tools.Tool{
		fs.NewReadTool(executor),
		fs.NewGlobTool(executor),
		fs.NewGrepTool(executor),
	}
}

// normalizeDirectArguments validates every filesystem path a direct tool
// accepts, confines it to root, and rewrites it to a root-relative spelling
// before the filesystem tool sees it. LocalExecutor deliberately is not a
// security jail, so this adapter owns the direct-call trust boundary.
func normalizeDirectArguments(root, name, arguments string) (string, error) {
	switch name {
	case "read":
		var request fs.ReadRequest
		if err := json.Unmarshal([]byte(arguments), &request); err != nil {
			return "", fmt.Errorf("toolset: decode direct read arguments: %w", err)
		}
		path, err := directPath(root, request.FilePath)
		if err != nil {
			return "", err
		}
		request.FilePath = path
		return encodeDirectArguments(request)
	case "glob":
		var request fs.GlobRequest
		if err := json.Unmarshal([]byte(arguments), &request); err != nil {
			return "", fmt.Errorf("toolset: decode direct glob arguments: %w", err)
		}
		if err := validateDirectGlobPattern(request.Pattern); err != nil {
			return "", err
		}
		if request.Path != "" {
			path, err := directPath(root, request.Path)
			if err != nil {
				return "", err
			}
			request.Path = path
		}
		return encodeDirectArguments(request)
	case "grep":
		var request fs.GrepRequest
		if err := json.Unmarshal([]byte(arguments), &request); err != nil {
			return "", fmt.Errorf("toolset: decode direct grep arguments: %w", err)
		}
		if request.Path != "" {
			path, err := directPath(root, request.Path)
			if err != nil {
				return "", err
			}
			request.Path = path
		}
		return encodeDirectArguments(request)
	default:
		return "", fmt.Errorf("toolset: direct tool %q is not registered", name)
	}
}

// validateDirectGlobPattern closes the second path channel accepted by glob.
// Glob.Path chooses a root, but Pattern also contributes the find anchor (for
// example "../**/*"), so checking Path alone would leave a root escape.
func validateDirectGlobPattern(pattern string) error {
	if filepath.IsAbs(pattern) {
		return fmt.Errorf("%w: absolute glob pattern %q", workspaceapp.ErrPathOutsideRoot, pattern)
	}
	for _, segment := range strings.FieldsFunc(pattern, func(r rune) bool {
		return r == '/' || r == filepath.Separator
	}) {
		if segment == ".." {
			return fmt.Errorf("%w: glob pattern %q", workspaceapp.ErrPathOutsideRoot, pattern)
		}
	}
	return nil
}

func encodeDirectArguments(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("toolset: encode direct arguments: %w", err)
	}
	return string(encoded), nil
}

func directPath(root, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%w: path is required", workspaceapp.ErrPathRequired)
	}
	// Resolve both values first. On macOS, temporary directories commonly have
	// a lexical /var/... spelling but a physical /private/var/... spelling;
	// comparing only a resolved target to an unresolved root would reject an
	// in-root file (or make the policy platform-dependent).
	resolvedRoot, err := pathidentity.Resolve("", root)
	if err != nil {
		return "", fmt.Errorf("%w: resolve root %q: %w", workspaceapp.ErrPathOutsideRoot, root, err)
	}
	resolved, err := pathidentity.Resolve(resolvedRoot, path)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %w", workspaceapp.ErrPathOutsideRoot, path, err)
	}
	inside, err := pathidentity.Contains(resolvedRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("%w: compare %q: %w", workspaceapp.ErrPathOutsideRoot, path, err)
	}
	if !inside {
		return "", fmt.Errorf("%w: %q", workspaceapp.ErrPathOutsideRoot, path)
	}
	relative, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", workspaceapp.ErrPathOutsideRoot, path, err)
	}
	return relative, nil
}

// directResult preserves a tool's structured JSON output when present and
// otherwise exposes its raw textual result as a JSON string, matching the
// protocol's best-effort JSON contract.
func directResult(output string) tool.Result {
	if result, err := tool.ParseResult([]byte(output)); err == nil {
		return result
	}
	return tool.StringResult(output)
}
