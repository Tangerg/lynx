package capabilityflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

const knowledgeFileName = "LYRA.md"

type KnowledgeDocument struct {
	Path    string
	Content string
}

type knowledgeDocument struct {
	scope     protocol.KnowledgeScope
	anchor    string
	relative  string
	path      string
	source    string
	directory fs.FileMode
	file      fs.FileMode
}

func (service *Service) ListKnowledge(
	ctx context.Context,
	query protocol.WorkspaceQuery,
) (*protocol.Page[protocol.KnowledgeEntry], error) {
	documents, err := service.knowledgeCascade(ctx, &query.Workspace)
	if err != nil {
		return nil, err
	}
	values := make([]protocol.KnowledgeEntry, 0, len(documents))
	for _, document := range documents {
		entry, err := readKnowledgeDocument(ctx, document)
		if err != nil {
			return nil, err
		}
		values = append(values, entry)
	}
	return protocol.NewPage(values), nil
}

func (service *Service) GetKnowledge(
	ctx context.Context,
	request protocol.GetKnowledgeRequest,
) (*protocol.KnowledgeEntry, error) {
	document, err := service.knowledgeDocument(ctx, request.Scope, request.Workspace)
	if err != nil {
		return nil, err
	}
	entry, err := readKnowledgeDocument(ctx, document)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (service *Service) UpdateKnowledge(
	ctx context.Context,
	request protocol.UpdateKnowledgeRequest,
) (*protocol.KnowledgeEntry, error) {
	if strings.TrimSpace(request.ExpectedRevision) == "" {
		return nil, fmt.Errorf("%w: expectedRevision is required", protocol.ErrInvalidParams)
	}
	content := []byte(request.Content)
	if len(content) > maxAuthoredDocumentBytes {
		return nil, fmt.Errorf("%w: knowledge exceeds %d bytes", protocol.ErrInvalidParams, maxAuthoredDocumentBytes)
	}
	for {
		document, err := service.knowledgeDocument(ctx, request.Scope, request.Workspace)
		if err != nil {
			return nil, err
		}
		release, err := service.serial.Acquire(ctx, "knowledge\x00"+document.path)
		if err != nil {
			return nil, err
		}
		currentDocument, err := service.knowledgeDocument(ctx, request.Scope, request.Workspace)
		if err != nil {
			release()
			return nil, err
		}
		if currentDocument.path != document.path {
			release()
			continue
		}
		entry, err := replaceKnowledgeDocument(
			ctx, currentDocument, request.ExpectedRevision, content,
		)
		release()
		if err != nil {
			return nil, err
		}
		return &entry, nil
	}
}

// KnowledgeDocuments exposes the effective broad-to-specific prompt cascade
// without leaking file layout into the Agent executor.
func (service *Service) KnowledgeDocuments(
	ctx context.Context,
	workspace string,
) ([]KnowledgeDocument, error) {
	documents, err := service.knowledgeCascade(
		ctx, &protocol.WorkspaceRef{Path: workspace},
	)
	if err != nil {
		return nil, err
	}
	values := make([]KnowledgeDocument, 0, len(documents))
	for _, document := range documents {
		entry, err := readKnowledgeDocument(ctx, document)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(entry.Content) == "" {
			continue
		}
		values = append(values, KnowledgeDocument{
			Path: document.source, Content: entry.Content,
		})
	}
	return values, nil
}

// KnowledgeFiles supplies Runtime observation with the exact documents whose
// external edits invalidate the global knowledge query family.
func (service *Service) KnowledgeFiles(
	ctx context.Context,
	workspaces []protocol.WorkspaceRef,
) ([]string, error) {
	home, err := service.knowledgeDocument(ctx, protocol.KnowledgeScopeHome, nil)
	if err != nil {
		return nil, err
	}
	paths := []string{home.source, home.path}
	for _, workspace := range workspaces {
		documents, err := service.knowledgeCascade(ctx, &workspace)
		if err != nil {
			return nil, err
		}
		for _, document := range documents {
			paths = append(paths, document.source, document.path)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

func (service *Service) knowledgeCascade(
	ctx context.Context,
	workspace *protocol.WorkspaceRef,
) ([]knowledgeDocument, error) {
	resolved, err := service.resolve(ctx, workspace)
	if err != nil {
		return nil, err
	}
	home, err := service.knowledgeDocumentForResolution(
		protocol.KnowledgeScopeHome, resolved,
	)
	if err != nil {
		return nil, err
	}
	project, err := service.knowledgeDocumentForResolution(
		protocol.KnowledgeScopeProjectRoot, resolved,
	)
	if err != nil {
		return nil, err
	}
	cwd, err := service.knowledgeDocumentForResolution(
		protocol.KnowledgeScopeCWD, resolved,
	)
	if err != nil {
		return nil, err
	}
	values := []knowledgeDocument{home}
	if project.path != cwd.path {
		values = append(values, project)
	}
	return append(values, cwd), nil
}

func (service *Service) knowledgeDocument(
	ctx context.Context,
	scope protocol.KnowledgeScope,
	workspace *protocol.WorkspaceRef,
) (knowledgeDocument, error) {
	if !scope.Valid() {
		return knowledgeDocument{}, fmt.Errorf("%w: invalid knowledge scope", protocol.ErrInvalidParams)
	}
	if scope == protocol.KnowledgeScopeHome {
		return newKnowledgeDocument(
			scope, service.home, filepath.Join(".lyra", knowledgeFileName),
		)
	}
	resolved, err := service.resolve(ctx, workspace)
	if err != nil {
		return knowledgeDocument{}, err
	}
	return service.knowledgeDocumentForResolution(scope, resolved)
}

func (service *Service) knowledgeDocumentForResolution(
	scope protocol.KnowledgeScope,
	resolved workspacefs.Resolution,
) (knowledgeDocument, error) {
	if scope == protocol.KnowledgeScopeHome {
		return newKnowledgeDocument(
			scope, service.home, filepath.Join(".lyra", knowledgeFileName),
		)
	}
	anchor := resolved.Workspace.Path()
	if scope == protocol.KnowledgeScopeProjectRoot && resolved.ProjectRoot != "" {
		anchor = resolved.ProjectRoot
	}
	return newKnowledgeDocument(scope, anchor, knowledgeFileName)
}

func newKnowledgeDocument(
	scope protocol.KnowledgeScope,
	anchor string,
	relative string,
) (knowledgeDocument, error) {
	target, err := confinedAuthoredTarget(anchor, relative)
	if err != nil {
		return knowledgeDocument{}, err
	}
	directoryMode := fs.FileMode(0o755)
	fileMode := fs.FileMode(0o644)
	if scope == protocol.KnowledgeScopeHome {
		directoryMode = 0o700
		fileMode = 0o600
	}
	return knowledgeDocument{
		scope: scope, anchor: target.anchor, relative: target.relative,
		path: target.path, source: filepath.Join(anchor, filepath.Clean(relative)),
		directory: directoryMode, file: fileMode,
	}, nil
}

func readKnowledgeDocument(
	ctx context.Context,
	document knowledgeDocument,
) (protocol.KnowledgeEntry, error) {
	if err := ctx.Err(); err != nil {
		return protocol.KnowledgeEntry{}, err
	}
	entry := protocol.KnowledgeEntry{
		Scope: document.scope, Revision: knowledgeRevision(nil),
	}
	root, err := os.OpenRoot(document.anchor)
	if err != nil {
		return protocol.KnowledgeEntry{}, fmt.Errorf("capabilityflow: open knowledge root: %w", err)
	}
	defer root.Close()
	file, err := root.Open(document.relative)
	if errors.Is(err, fs.ErrNotExist) {
		return entry, nil
	}
	if err != nil {
		return protocol.KnowledgeEntry{}, fmt.Errorf("capabilityflow: open knowledge document: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return protocol.KnowledgeEntry{}, err
	}
	data, err := readBoundedAuthoredFile(file, info)
	if err != nil {
		return protocol.KnowledgeEntry{}, fmt.Errorf("capabilityflow: read knowledge document: %w", err)
	}
	entry.Content = string(data)
	entry.Revision = knowledgeRevision(data)
	entry.UpdatedAt = info.ModTime()
	return entry, nil
}

func replaceKnowledgeDocument(
	ctx context.Context,
	document knowledgeDocument,
	expectedRevision string,
	content []byte,
) (protocol.KnowledgeEntry, error) {
	current, err := readKnowledgeDocument(ctx, document)
	if err != nil {
		return protocol.KnowledgeEntry{}, err
	}
	if current.Revision != expectedRevision {
		return protocol.KnowledgeEntry{}, protocol.ErrRevisionConflict
	}
	root, err := os.OpenRoot(document.anchor)
	if err != nil {
		return protocol.KnowledgeEntry{}, err
	}
	defer root.Close()
	directory := filepath.Dir(document.relative)
	if directory != "." {
		if err := root.MkdirAll(directory, document.directory); err != nil {
			return protocol.KnowledgeEntry{}, err
		}
	}
	temporaryName := filepath.Join(directory, ".LYRA.md.lyra-stage-"+rand.Text())
	temporary, err := root.OpenFile(
		temporaryName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, document.file,
	)
	if err != nil {
		return protocol.KnowledgeEntry{}, err
	}
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = root.Remove(temporaryName)
		}
	}()
	if _, err := temporary.Write(content); err != nil {
		return protocol.KnowledgeEntry{}, err
	}
	if err := temporary.Chmod(document.file); err != nil {
		return protocol.KnowledgeEntry{}, err
	}
	if err := temporary.Sync(); err != nil {
		return protocol.KnowledgeEntry{}, err
	}
	if err := temporary.Close(); err != nil {
		return protocol.KnowledgeEntry{}, err
	}
	if err := root.Rename(temporaryName, document.relative); err != nil {
		return protocol.KnowledgeEntry{}, err
	}
	committed = true
	updatedAt := time.Time{}
	if info, statErr := root.Stat(document.relative); statErr == nil {
		updatedAt = info.ModTime()
	}
	syncKnowledgeDirectory(root, directory)
	return protocol.KnowledgeEntry{
		Scope: document.scope, Content: string(content),
		Revision: knowledgeRevision(content), UpdatedAt: updatedAt,
	}, nil
}

func knowledgeRevision(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func syncKnowledgeDirectory(root *os.Root, directory string) {
	opened, err := root.Open(directory)
	if err != nil {
		return
	}
	_ = opened.Sync()
	_ = opened.Close()
}
