package toolset

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	toolcontract "github.com/Tangerg/scope/core/tool"

	workspaceapp "github.com/Tangerg/scope/app/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/infra/pathidentity"
	"github.com/Tangerg/scope/tools/fs"
)

// directTools is the small, read-only capability set valid without an agent
// process. Keep this list explicit: being available to a model does not make a
// tool valid for a client-driven call.
func directTools(root string) []toolcontract.Tool {
	executor := fs.NewLocalExecutor(root)
	search := newRuntimeSearchTools(root)
	return []toolcontract.Tool{newRuntimeReadTool(root, executor), search.glob, search.grep}
}

// normalizeDirectArguments validates the direct-call protocol's paths and
// rewrites them to the root-relative identity promised by the application
// contract. LocalExecutor independently enforces the filesystem capability;
// this adapter owns domain-error translation and protocol normalization.
func normalizeDirectArguments(root, name, arguments string) (string, error) {
	switch name {
	case tool.Read:
		request, err := decodeToolArguments[fs.ReadRequest](arguments)
		if err != nil {
			return "", fmt.Errorf("toolset: decode direct read arguments: %w", err)
		}
		path, err := directPath(root, request.Path)
		if err != nil {
			return "", err
		}
		request.Path = path
		return encodeDirectArguments(request)
	case tool.Glob:
		request, err := decodeToolArguments[runtimeGlobRequest](arguments)
		if err != nil {
			return "", fmt.Errorf("toolset: decode direct glob arguments: %w", err)
		}
		if request.Path != "" {
			path, err := directPath(root, request.Path)
			if err != nil {
				return "", err
			}
			request.Path = path
		}
		return encodeDirectArguments(request)
	case tool.Grep:
		request, err := decodeToolArguments[runtimeGrepRequest](arguments)
		if err != nil {
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

func decodeToolArguments[T any](arguments string) (T, error) {
	var request T
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		if err == nil {
			return request, errors.New("unexpected trailing JSON value")
		}
		return request, err
	}
	return request, nil
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
