package runtimeembedded

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

type workspaceBindingStub struct {
	resolved *protocol.WorkspaceInfo
	known    *protocol.Page[protocol.WorkspaceSummary]
	changes  *protocol.Page[protocol.WorkspaceFileChange]
	diff     *protocol.Diff
	head     *protocol.FileHead
	search   *protocol.GrepResult
	files    *protocol.Page[protocol.FileEntry]
	content  *protocol.FileContent
}

func (stub *workspaceBindingStub) ResolveWorkspace(context.Context, protocol.ResolveWorkspaceRequest, embedded.CallOptions) (*protocol.WorkspaceInfo, error) {
	return stub.resolved, nil
}

func (stub *workspaceBindingStub) ListWorkspaces(context.Context, embedded.CallOptions) (*protocol.Page[protocol.WorkspaceSummary], error) {
	return stub.known, nil
}

func (stub *workspaceBindingStub) ListWorkspaceFileChanges(context.Context, protocol.WorkspaceQuery, embedded.CallOptions) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	return stub.changes, nil
}

func (stub *workspaceBindingStub) GetWorkspaceDiff(context.Context, protocol.GetDiffRequest, embedded.CallOptions) (*protocol.Diff, error) {
	return stub.diff, nil
}

func (stub *workspaceBindingStub) GetWorkspaceFileHead(context.Context, protocol.GetFileHeadRequest, embedded.CallOptions) (*protocol.FileHead, error) {
	return stub.head, nil
}

func (stub *workspaceBindingStub) SearchWorkspaceFiles(context.Context, protocol.GrepRequest, embedded.CallOptions) (*protocol.GrepResult, error) {
	return stub.search, nil
}

func (stub *workspaceBindingStub) ListWorkspaceFiles(context.Context, protocol.ListFilesRequest, embedded.CallOptions) (*protocol.Page[protocol.FileEntry], error) {
	return stub.files, nil
}

func (stub *workspaceBindingStub) ReadWorkspaceFile(context.Context, protocol.ReadFileRequest, embedded.CallOptions) (*protocol.FileContent, error) {
	return stub.content, nil
}

func TestWorkspaceAdapterProjectsEveryReadShape(t *testing.T) {
	t.Parallel()
	added, removed, size := 4, 1, int64(120)
	stub := &workspaceBindingStub{
		resolved: &protocol.WorkspaceInfo{Ref: protocol.WorkspaceRef{Path: "/workspace"}, ProjectRoot: "/workspace", Availability: protocol.WorkspaceAvailable},
		known: protocol.NewPage([]protocol.WorkspaceSummary{{
			Workspace: protocol.WorkspaceInfo{Ref: protocol.WorkspaceRef{Path: "/workspace"}, ProjectRoot: "/workspace", Availability: protocol.WorkspaceAvailable},
			Name:      "workspace", SessionCount: 2,
		}}),
		changes: protocol.NewPage([]protocol.WorkspaceFileChange{{Path: "main.go", Status: protocol.FileStatusModified, Added: &added, Removed: &removed}}),
		diff: &protocol.Diff{Files: []protocol.FileDiff{{
			Path: "main.go", Status: protocol.FileStatusModified, Added: &added, Removed: &removed,
			Rows: []protocol.DiffRow{{Type: protocol.DiffRowAdded, RightLine: 1, Code: "package main"}},
		}}},
		head:    &protocol.FileHead{Path: "main.go", Lines: []protocol.FileLine{{LineNumber: 1, Text: "package main"}}},
		search:  &protocol.GrepResult{Matches: []protocol.GrepMatch{{Path: "main.go", LineNumber: 1, Text: "package main"}}, Total: 1},
		files:   protocol.NewPageWithCursor([]protocol.FileEntry{{Path: "main.go", Name: "main.go", Type: protocol.FileEntryFile, SizeBytes: &size, ModifiedAt: "2026-08-12T00:00:00Z"}}, "next"),
		content: &protocol.FileContent{Path: "main.go", Content: "package main\n", Encoding: "utf-8", TotalLines: 1},
	}
	runtime := &Runtime{workspaces: stub, meta: requestMeta("test")}

	resolved, err := runtime.Resolve(t.Context(), workspace.ResolveRequest{Path: "/workspace"})
	if err != nil || resolved.Path != "/workspace" || !resolved.IsAvailable() {
		t.Fatalf("Resolve = (%+v, %v)", resolved, err)
	}
	known, err := runtime.List(t.Context())
	if err != nil || len(known) != 1 || known[0].Sessions != 2 {
		t.Fatalf("List = (%+v, %v)", known, err)
	}
	changes, err := runtime.Changes(t.Context(), "/workspace")
	if err != nil || len(changes) != 1 || changes[0].Stat() != "+4 -1" {
		t.Fatalf("Changes = (%+v, %v)", changes, err)
	}
	diff, err := runtime.Diff(t.Context(), workspace.DiffRequest{Workspace: "/workspace", Format: workspace.DiffFormatRows})
	if err != nil || diff.Text() != "diff -- main.go (modified)\n+package main" {
		t.Fatalf("Diff = (%+v, %v)", diff, err)
	}
	head, err := runtime.Head(t.Context(), workspace.HeadRequest{Workspace: "/workspace", Path: "main.go"})
	if err != nil || len(head.Lines) != 1 {
		t.Fatalf("Head = (%+v, %v)", head, err)
	}
	search, err := runtime.Search(t.Context(), workspace.SearchRequest{Workspace: "/workspace", Query: "main"})
	if err != nil || search.Total != 1 || len(search.Matches) != 1 {
		t.Fatalf("Search = (%+v, %v)", search, err)
	}
	files, err := runtime.Files(t.Context(), workspace.FilesRequest{Workspace: "/workspace"})
	if err != nil || files.NextCursor != "next" || files.Entries[0].Type != workspace.FileEntryFile || *files.Entries[0].SizeBytes != size {
		t.Fatalf("Files = (%+v, %v)", files, err)
	}
	content, err := runtime.Read(t.Context(), workspace.ReadRequest{Workspace: "/workspace", Path: "main.go"})
	if err != nil || content.Content != "package main\n" || content.Window() != "1 lines" {
		t.Fatalf("Read = (%+v, %v)", content, err)
	}

	added = 99
	if *changes[0].Added != 4 {
		t.Fatal("workspace projection retained a mutable protocol pointer")
	}
}

func TestWorkspaceAdapterRejectsNilResponses(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{workspaces: &workspaceBindingStub{}, meta: requestMeta("test")}
	if _, err := runtime.Read(t.Context(), workspace.ReadRequest{Workspace: "/workspace", Path: "main.go"}); err == nil {
		t.Fatal("nil file content was accepted")
	}
}
