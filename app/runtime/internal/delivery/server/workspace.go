package server

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// workspace.* (API.md §7.5) is split across files by the source each read draws
// from — this file holds only the shared path helpers; the method groups live
// alongside their backend:
//
//	workspace_vcs.go        git: listFileChanges / getDiff
//	workspace_fs.go         filesystem: getFileHead / grep
//	workspace_discovery.go  derived: listProjects / listSkills / listAgentDocs
//	workspace_mcp.go        MCP: mcp.listServers / listTools / reconnect
//	workspace_stream.go     workspace.subscribe + the file watcher

// workspaceRoot resolves the effective root for a workspace read: the
// request's cwd, or the serve directory when omitted (API.md §7.5 "default =
// serve directory"). It returns ErrCwdUnavailable when the root doesn't
// resolve to an existing directory, so reads against a stale cwd fail
// honestly rather than returning empty.
func (s *Server) workspaceRoot(cwd string) (string, error) {
	root := cwd
	if root == "" {
		root = s.serverInfo.Cwd
	}
	resolved, err := workspacepath.ResolveExistingDir(root)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", protocol.ErrCwdUnavailable, root, err)
	}
	return resolved, nil
}

// resolveInRoot lexically confines a client-supplied path to root and returns
// it relative to root. Absolute paths are accepted only when already inside
// root; anything climbing out is rejected as path_outside_root (API.md §7.5).
func resolveInRoot(root, p string) (rel string, err error) {
	if p == "" {
		return "", fmt.Errorf("%w: path required", protocol.ErrInvalidParams)
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, p)
	}
	abs = filepath.Clean(abs)
	rel, err = filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", protocol.ErrPathOutsideRoot
	}
	return rel, nil
}

// resolveExistingInRoot applies the lexical jail and then resolves symlinks for
// paths that exist, so filesystem reads cannot escape through a symlink rooted
// inside the workspace. Missing paths are left to the caller's backend error.
func resolveExistingInRoot(root, p string) (string, error) {
	rel, err := resolveInRoot(root, p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, rel))
	if err != nil {
		return rel, nil
	}
	resolved = workspacepath.Canonical(resolved)
	if !pathInside(workspacepath.Canonical(root), resolved) {
		return "", protocol.ErrPathOutsideRoot
	}
	return rel, nil
}

func pathInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
