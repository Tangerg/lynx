package capabilityflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

// AgentDocument is the complete Runtime-owned representation shared by the
// discovery operation and fresh Run construction. Content intentionally stays
// off the agentDocs.list wire surface.
type AgentDocument struct {
	Path    string
	Title   string
	Scope   protocol.AgentDocScope
	Content string
}

type discoveredAgentDocument struct {
	AgentDocument
	info fs.FileInfo
}

func (service *Service) AgentDocs(
	ctx context.Context,
	query protocol.WorkspaceQuery,
) (*protocol.Page[protocol.AgentDoc], error) {
	values, err := service.AgentDocuments(ctx, query.Workspace.Path)
	if err != nil {
		return nil, err
	}
	wire := make([]protocol.AgentDoc, 0, len(values))
	for _, value := range values {
		wire = append(wire, protocol.AgentDoc{
			Path: value.Path, Title: value.Title, Scope: value.Scope,
		})
	}
	return protocol.NewPage(wire), nil
}

// AgentDocuments discovers the exact root-to-leaf instruction set applicable
// to a workspace. An os.Root confines every read to its declared owner even
// when an authored path contains a symlink.
func (service *Service) AgentDocuments(
	ctx context.Context,
	workspace string,
) ([]AgentDocument, error) {
	resolved, err := service.resolve(ctx, &protocol.WorkspaceRef{Path: workspace})
	if err != nil {
		return nil, err
	}
	cwd := resolved.Workspace.Path()
	projectRoot := resolved.ProjectRoot
	if projectRoot == "" {
		projectRoot = cwd
	}
	directories, err := containedRootToLeaf(projectRoot, cwd)
	if err != nil {
		return nil, err
	}
	discovered := make([]discoveredAgentDocument, 0)
	homeRoot, err := os.OpenRoot(service.home)
	if err != nil {
		return nil, fmt.Errorf("capabilityflow: open home agent-doc root: %w", err)
	}
	if err := appendFirstAgentDocument(
		ctx, &discovered, homeRoot, service.home, protocol.AgentDocScopeHome,
		".lyra/AGENTS.md",
	); err != nil {
		_ = homeRoot.Close()
		return nil, err
	}
	if err := appendFirstAgentDocument(
		ctx, &discovered, homeRoot, service.home, protocol.AgentDocScopeHome,
		".agents/AGENTS.md", ".agents/agents.md",
	); err != nil {
		_ = homeRoot.Close()
		return nil, err
	}
	if err := homeRoot.Close(); err != nil {
		return nil, err
	}
	project, err := os.OpenRoot(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("capabilityflow: open project agent-doc root: %w", err)
	}
	defer project.Close()
	for _, directory := range directories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(projectRoot, directory)
		if err != nil {
			return nil, err
		}
		scope := protocol.AgentDocScopeProjectRoot
		if directory == cwd {
			scope = protocol.AgentDocScopeCWD
		}
		if err := appendFirstAgentDocument(
			ctx, &discovered, project, projectRoot, scope,
			filepath.Join(relative, ".lyra", "AGENTS.md"),
		); err != nil {
			return nil, err
		}
		if err := appendFirstAgentDocument(
			ctx, &discovered, project, projectRoot, scope,
			filepath.Join(relative, "AGENTS.md"), filepath.Join(relative, "agents.md"),
		); err != nil {
			return nil, err
		}
	}
	values := make([]AgentDocument, 0, len(discovered))
	for _, value := range discovered {
		values = append(values, value.AgentDocument)
	}
	return values, nil
}

func appendFirstAgentDocument(
	ctx context.Context,
	values *[]discoveredAgentDocument,
	root *os.Root,
	anchor string,
	scope protocol.AgentDocScope,
	candidates ...string,
) error {
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, err := root.Open(candidate)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("capabilityflow: open agent document %s: %w", candidate, err)
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return err
		}
		data, err := readBoundedAuthoredFile(file, info)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			return nil
		}
		for _, existing := range *values {
			if os.SameFile(existing.info, info) {
				return nil
			}
		}
		path := filepath.Join(anchor, filepath.Clean(candidate))
		*values = append(*values, discoveredAgentDocument{
			AgentDocument: AgentDocument{
				Path: path, Title: agentDocumentTitle(content, path),
				Scope: scope, Content: content,
			},
			info: info,
		})
		return nil
	}
	return nil
}

func agentDocumentTitle(content string, path string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return filepath.Base(path)
}

func containedRootToLeaf(root string, leaf string) ([]string, error) {
	root = filepath.Clean(root)
	leaf = filepath.Clean(leaf)
	relative, err := filepath.Rel(root, leaf)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, protocol.ErrPathOutsideRoot
	}
	values := make([]string, 0)
	for current := leaf; ; current = filepath.Dir(current) {
		values = append(values, current)
		if current == root {
			break
		}
	}
	slices.Reverse(values)
	return values, nil
}
