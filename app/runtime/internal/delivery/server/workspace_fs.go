package server

import (
	"context"
	"fmt"
	"time"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// ListWorkspaceFiles projects a paged application workspace-file listing onto
// the wire contract.
func (s *Server) ListWorkspaceFiles(ctx context.Context, in protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error) {
	page, err := s.workspaceFiles.ListFiles(ctx, workspaceapp.FileListInput{
		Cwd: in.Cwd,
		FileListOptions: workspaceapp.FileListOptions{
			Path: in.Path, Glob: in.Glob, Recursive: in.Recursive, IncludeIgnored: in.IncludeIgnored,
		},
		Cursor: in.Cursor,
		Limit:  in.Limit,
	})
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	data := make([]protocol.FileEntry, 0, len(page.Entries))
	for _, entry := range page.Entries {
		kind, ok := fileEntryTypeWire(entry.Kind)
		if !ok {
			return nil, fmt.Errorf("workspace.listFiles: unsupported entry kind %q", entry.Kind)
		}
		var sizeBytes *int64
		if entry.Kind == workspaceapp.FileEntryFile {
			sizeBytes = &entry.SizeBytes
		}
		data = append(data, protocol.FileEntry{
			Path: entry.Path, Name: entry.Name, Type: kind, SizeBytes: sizeBytes,
			ModifiedAt: entry.ModifiedAt.Format(time.RFC3339Nano),
		})
	}
	return &protocol.Page[protocol.FileEntry]{Data: data, NextCursor: page.NextCursor}, nil
}

func fileEntryTypeWire(kind workspaceapp.FileEntryKind) (protocol.FileEntryType, bool) {
	switch kind {
	case workspaceapp.FileEntryFile:
		return protocol.FileEntryFile, true
	case workspaceapp.FileEntryDir:
		return protocol.FileEntryDir, true
	case workspaceapp.FileEntrySymlink:
		return protocol.FileEntrySymlink, true
	default:
		return "", false
	}
}

// GetWorkspaceFileHead projects the application file preview onto wire lines.
func (s *Server) GetWorkspaceFileHead(ctx context.Context, in protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
	head, err := s.workspaceFiles.FileHead(ctx, in.Cwd, in.Path, in.Lines)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	lines := make([]protocol.FileLine, 0, len(head.Lines))
	for _, line := range head.Lines {
		lines = append(lines, protocol.FileLine{LineNumber: line.Number, Text: line.Text})
	}
	return &protocol.FileHead{Path: in.Path, Lines: lines}, nil
}

// ReadWorkspaceFile maps the application file read onto the protocol response.
func (s *Server) ReadWorkspaceFile(ctx context.Context, in protocol.ReadFileRequest) (*protocol.FileContent, error) {
	read, err := s.workspaceFiles.ReadFile(ctx, in.Cwd, workspaceapp.FileReadInput{
		Path: in.Path, MaxBytes: in.MaxBytes, StartLine: in.StartLine, EndLine: in.EndLine,
	})
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := &protocol.FileContent{
		Path: in.Path, Content: read.Content, Encoding: "utf-8", TotalLines: read.TotalLines, Truncated: read.Truncated,
	}
	if in.StartLine > 0 {
		out.StartLine = read.StartLine + 1
		out.EndLine = read.EndLine
	}
	return out, nil
}

// GrepWorkspace maps the application content search onto the protocol result.
func (s *Server) GrepWorkspace(ctx context.Context, in protocol.GrepRequest) (*protocol.GrepResult, error) {
	result, err := s.workspaceFiles.Grep(ctx, in.Cwd, workspaceapp.GrepInput{Path: in.Path, Query: in.Query, Limit: in.Limit})
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	matches := make([]protocol.GrepMatch, 0, len(result.Matches))
	for _, match := range result.Matches {
		matches = append(matches, protocol.GrepMatch{Path: match.Path, LineNumber: match.LineNumber, Text: match.Text})
	}
	return &protocol.GrepResult{Matches: matches, Total: result.Total}, nil
}
