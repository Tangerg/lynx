package runtimeembedded

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

type workspaceBinding interface {
	ResolveWorkspace(context.Context, protocol.ResolveWorkspaceRequest, embedded.CallOptions) (*protocol.WorkspaceInfo, error)
	ListWorkspaces(context.Context, embedded.CallOptions) (*protocol.Page[protocol.WorkspaceSummary], error)
	ListWorkspaceFileChanges(context.Context, protocol.WorkspaceQuery, embedded.CallOptions) (*protocol.Page[protocol.WorkspaceFileChange], error)
	GetWorkspaceDiff(context.Context, protocol.GetDiffRequest, embedded.CallOptions) (*protocol.Diff, error)
	GetWorkspaceFileHead(context.Context, protocol.GetFileHeadRequest, embedded.CallOptions) (*protocol.FileHead, error)
	SearchWorkspaceFiles(context.Context, protocol.GrepRequest, embedded.CallOptions) (*protocol.GrepResult, error)
	ListWorkspaceFiles(context.Context, protocol.ListFilesRequest, embedded.CallOptions) (*protocol.Page[protocol.FileEntry], error)
	ReadWorkspaceFile(context.Context, protocol.ReadFileRequest, embedded.CallOptions) (*protocol.FileContent, error)
}

const workspaceFilePageLimit = 500

func (r *Runtime) Resolve(ctx context.Context, request workspace.ResolveRequest) (workspace.Workspace, error) {
	if err := request.Validate(); err != nil {
		return workspace.Workspace{}, err
	}
	wire := protocol.ResolveWorkspaceRequest{}
	if request.Path != "" {
		wire.Ref = &protocol.WorkspaceRef{Path: request.Path}
	}
	resolved, err := r.workspaces.ResolveWorkspace(ctx, wire, r.callOptions())
	if err != nil {
		return workspace.Workspace{}, classifyError(err)
	}
	if resolved == nil {
		return workspace.Workspace{}, runtimeContractViolation("resolve workspace returned nil")
	}
	projected, err := projectWorkspace(*resolved)
	if err != nil {
		return workspace.Workspace{}, runtimeContractViolation("resolve workspace returned an invalid workspace: %v", err)
	}
	return projected, nil
}

func (r *Runtime) List(ctx context.Context) ([]workspace.Summary, error) {
	page, err := r.workspaces.ListWorkspaces(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list workspaces", page)
	if err != nil {
		return nil, err
	}
	result := make([]workspace.Summary, 0, len(values))
	for index, value := range values {
		projected, err := projectWorkspaceSummary(value)
		if err != nil {
			return nil, runtimeContractViolation("list workspaces row %d is invalid: %v", index, err)
		}
		result = append(result, projected)
	}
	return result, nil
}

func (r *Runtime) Changes(ctx context.Context, path string) ([]workspace.Change, error) {
	if err := r.requireFeature(runtimeprofile.FeatureGit); err != nil {
		return nil, err
	}
	page, err := r.workspaces.ListWorkspaceFileChanges(ctx, protocol.WorkspaceQuery{
		Workspace: protocol.WorkspaceRef{Path: path},
	}, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list workspace changes", page)
	if err != nil {
		return nil, err
	}
	result := make([]workspace.Change, 0, len(values))
	for index, value := range values {
		projected, err := projectChange(value.Path, value.Status, value.PreviousPath, value.Added, value.Removed, value.Binary)
		if err != nil {
			return nil, runtimeContractViolation("list workspace changes row %d is invalid: %v", index, err)
		}
		result = append(result, projected)
	}
	return result, nil
}

func (r *Runtime) Diff(ctx context.Context, request workspace.DiffRequest) (workspace.Diff, error) {
	if err := request.Validate(); err != nil {
		return workspace.Diff{}, err
	}
	if err := r.requireFeature(runtimeprofile.FeatureGit); err != nil {
		return workspace.Diff{}, err
	}
	value, err := r.workspaces.GetWorkspaceDiff(ctx, protocol.GetDiffRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path,
		Mode: protocol.DiffMode(request.Mode), Format: protocol.DiffFormat(request.Format), Limit: request.Limit,
	}, r.callOptions())
	if err != nil {
		return workspace.Diff{}, classifyError(err)
	}
	if value == nil {
		return workspace.Diff{}, runtimeContractViolation("get workspace diff returned nil")
	}
	projected, err := projectDiff(*value)
	if err != nil {
		return workspace.Diff{}, runtimeContractViolation("get workspace diff returned an invalid projection: %v", err)
	}
	return projected, nil
}

func (r *Runtime) Head(ctx context.Context, request workspace.HeadRequest) (workspace.FileHead, error) {
	if err := request.Validate(); err != nil {
		return workspace.FileHead{}, err
	}
	value, err := r.workspaces.GetWorkspaceFileHead(ctx, protocol.GetFileHeadRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path, Lines: request.Lines,
	}, r.callOptions())
	if err != nil {
		return workspace.FileHead{}, classifyError(err)
	}
	if value == nil {
		return workspace.FileHead{}, runtimeContractViolation("get workspace file head returned nil")
	}
	result := workspace.FileHead{Path: value.Path, Lines: make([]workspace.FileLine, 0, len(value.Lines))}
	for _, line := range value.Lines {
		result.Lines = append(result.Lines, workspace.FileLine{Number: line.LineNumber, Text: line.Text})
	}
	if err := result.Validate(); err != nil {
		return workspace.FileHead{}, runtimeContractViolation("get workspace file head returned an invalid projection: %v", err)
	}
	return result, nil
}

func (r *Runtime) Search(ctx context.Context, request workspace.SearchRequest) (workspace.SearchResult, error) {
	if err := request.Validate(); err != nil {
		return workspace.SearchResult{}, err
	}
	value, err := r.workspaces.SearchWorkspaceFiles(ctx, protocol.GrepRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Query: request.Query,
		Path: request.Path, Limit: request.Limit,
	}, r.callOptions())
	if err != nil {
		return workspace.SearchResult{}, classifyError(err)
	}
	if value == nil {
		return workspace.SearchResult{}, runtimeContractViolation("search workspace files returned nil")
	}
	result := workspace.SearchResult{Total: value.Total, Matches: make([]workspace.Match, 0, len(value.Matches))}
	for _, match := range value.Matches {
		result.Matches = append(result.Matches, workspace.Match{Path: match.Path, Line: match.LineNumber, Text: match.Text})
	}
	if err := result.Validate(); err != nil {
		return workspace.SearchResult{}, runtimeContractViolation("search workspace files returned an invalid projection: %v", err)
	}
	return result, nil
}

func (r *Runtime) Files(ctx context.Context, request workspace.FilesRequest) (workspace.FileListing, error) {
	if err := request.Validate(); err != nil {
		return workspace.FileListing{}, err
	}
	result := workspace.FileListing{}
	cursors := newCursorTraversal("list workspace files", "")
	for {
		if err := context.Cause(ctx); err != nil {
			return workspace.FileListing{}, err
		}
		cursor := cursors.Current()
		page, err := r.workspaces.ListWorkspaceFiles(ctx, protocol.ListFilesRequest{
			Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path, Glob: request.Glob,
			Recursive: request.Recursive, IncludeIgnored: request.IncludeIgnored,
			PageQuery: protocol.PageQuery{Limit: workspaceFilePageLimit, Cursor: cursor},
		}, r.callOptions())
		if err != nil {
			return workspace.FileListing{}, classifyError(err)
		}
		if page == nil {
			return workspace.FileListing{}, runtimeContractViolation("list workspace files after cursor %q returned a nil page", cursor)
		}
		for _, entry := range page.Data {
			result.Entries = append(result.Entries, workspace.FileEntry{
				Path: entry.Path, Name: entry.Name, Type: projectFileType(entry.Type),
				SizeBytes: cloneInt64(entry.SizeBytes), ModifiedAt: entry.ModifiedAt,
			})
		}
		more, err := cursors.Advance(page.NextCursor)
		if err != nil {
			return workspace.FileListing{}, err
		}
		if !more {
			break
		}
	}
	if err := result.Validate(); err != nil {
		return workspace.FileListing{}, runtimeContractViolation("list workspace files returned an invalid projection: %v", err)
	}
	return result, nil
}

func (r *Runtime) Read(ctx context.Context, request workspace.ReadRequest) (workspace.FileContent, error) {
	if err := request.Validate(); err != nil {
		return workspace.FileContent{}, err
	}
	value, err := r.workspaces.ReadWorkspaceFile(ctx, protocol.ReadFileRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path,
		StartLine: request.StartLine, EndLine: request.EndLine, MaxBytes: request.MaxBytes,
	}, r.callOptions())
	if err != nil {
		return workspace.FileContent{}, classifyError(err)
	}
	if value == nil {
		return workspace.FileContent{}, runtimeContractViolation("read workspace file returned nil")
	}
	result := workspace.FileContent{
		Path: value.Path, Content: value.Content, Encoding: value.Encoding, TotalLines: value.TotalLines,
		Truncated: value.Truncated, StartLine: value.StartLine, EndLine: value.EndLine,
	}
	if err := result.Validate(); err != nil {
		return workspace.FileContent{}, runtimeContractViolation("read workspace file returned an invalid projection: %v", err)
	}
	return result, nil
}

func projectWorkspace(value protocol.WorkspaceInfo) (workspace.Workspace, error) {
	result := workspace.Workspace{
		Path: value.Ref.Path, ProjectRoot: value.ProjectRoot, Availability: workspace.Availability(value.Availability),
	}
	if err := result.Validate(); err != nil {
		return workspace.Workspace{}, fmt.Errorf("runtime workspace %q: %w", value.Ref.Path, err)
	}
	return result, nil
}

func projectWorkspaceSummary(value protocol.WorkspaceSummary) (workspace.Summary, error) {
	projected, err := projectWorkspace(value.Workspace)
	if err != nil {
		return workspace.Summary{}, err
	}
	result := workspace.Summary{
		Workspace: projected, Name: value.Name, Sessions: value.SessionCount,
	}
	if value.LastActiveAt != nil {
		result.LastActive = new(*value.LastActiveAt)
	}
	if err := result.Validate(); err != nil {
		return workspace.Summary{}, err
	}
	return result, nil
}

func projectChange(path string, status protocol.FileStatus, previousPath string, added, removed *int, binary bool) (workspace.Change, error) {
	result := workspace.Change{
		Path: path, Status: workspace.FileStatus(status), PreviousPath: previousPath,
		Added: cloneInt(added), Removed: cloneInt(removed), Binary: binary,
	}
	if err := result.Validate(); err != nil {
		return workspace.Change{}, err
	}
	return result, nil
}

func projectDiff(value protocol.Diff) (workspace.Diff, error) {
	result := workspace.Diff{Patch: value.Patch, Truncated: value.Truncated, Files: make([]workspace.FileDiff, 0, len(value.Files))}
	for index, file := range value.Files {
		change, err := projectChange(file.Path, file.Status, file.PreviousPath, file.Added, file.Removed, file.Binary)
		if err != nil {
			return workspace.Diff{}, fmt.Errorf("workspace file diff %d: %w", index, err)
		}
		rows := make([]workspace.DiffRow, 0, len(file.Rows))
		for _, row := range file.Rows {
			rows = append(rows, workspace.DiffRow{
				Type: projectDiffRowType(row.Type), Text: row.Text, LeftLine: row.LeftLine,
				RightLine: row.RightLine, Code: row.Code,
			})
		}
		result.Files = append(result.Files, workspace.FileDiff{Change: change, Rows: rows})
	}
	if err := result.Validate(); err != nil {
		return workspace.Diff{}, fmt.Errorf("get workspace diff projection: %w", err)
	}
	return result, nil
}

func projectDiffRowType(value protocol.DiffRowType) workspace.DiffRowType {
	switch value {
	case protocol.DiffRowHunk:
		return workspace.DiffRowHunk
	case protocol.DiffRowContext:
		return workspace.DiffRowContext
	case protocol.DiffRowAdded:
		return workspace.DiffRowAdded
	case protocol.DiffRowDeleted:
		return workspace.DiffRowDeleted
	default:
		return workspace.DiffRowType(value)
	}
}

func projectFileType(value protocol.FileEntryType) workspace.FileType {
	switch value {
	case protocol.FileEntryFile:
		return workspace.FileEntryFile
	case protocol.FileEntryDir:
		return workspace.FileEntryDirectory
	case protocol.FileEntrySymlink:
		return workspace.FileEntrySymlink
	default:
		return workspace.FileType(value)
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	return new(*value)
}
