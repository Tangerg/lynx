package server

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	workspaceadapter "github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

func newWorkspaceCoordinator(cwd string, cfg workspaceapp.Config) *workspaceapp.Coordinator {
	cfg.DefaultCwd = cwd
	cfg.Home = cwd
	cfg.Paths = workspacepath.Resolver{}
	cfg.Files = workspaceadapter.Reads{}
	cfg.Git = workspaceadapter.VCS{}
	return workspaceapp.New(cfg)
}

func newWorkspaceServer(cwd string) *Server {
	return &Server{workspace: newWorkspaceCoordinator(cwd, workspaceapp.Config{})}
}

// TestWorkspaceGetFileHead reads the first N lines of a cwd-relative file,
// numbers them 1-based, and refuses a path that climbs out of the root.
func TestWorkspaceGetFileHead(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceServer(dir)

	got, err := s.WorkspaceGetFileHead(context.Background(), protocol.GetFileHeadRequest{Path: "f.txt", Lines: 2})
	if err != nil {
		t.Fatalf("getFileHead: %v", err)
	}
	if len(got.Lines) != 2 || got.Lines[0].LineNumber != 1 || got.Lines[0].Text != "a" || got.Lines[1].LineNumber != 2 {
		t.Fatalf("lines = %+v, want first two lines numbered 1,2", got.Lines)
	}

	if _, err := s.WorkspaceGetFileHead(context.Background(), protocol.GetFileHeadRequest{Path: "../escape"}); !errors.Is(err, protocol.ErrPathOutsideRoot) {
		t.Errorf("escape path err = %v, want ErrPathOutsideRoot", err)
	}
}

func TestWorkspaceListFilesPaginatesInspectedEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := newWorkspaceServer(dir)

	first, err := s.WorkspaceListFiles(context.Background(), protocol.ListFilesRequest{Recursive: true, PageQuery: protocol.PageQuery{Limit: 2}})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Data) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two entries and a cursor", first)
	}
	if _, err := base64.RawURLEncoding.DecodeString(first.NextCursor); err != nil {
		t.Fatalf("cursor = %q, want opaque base64 key: %v", first.NextCursor, err)
	}
	if first.Data[0].Type != protocol.FileEntryFile || first.Data[0].SizeBytes == nil || *first.Data[0].SizeBytes == 0 || first.Data[0].ModifiedAt == "" {
		t.Fatalf("entry is not fully inspected: %+v", first.Data[0])
	}
	// The cursor is an ordered key rather than a row-existence dependency: if
	// its file disappears between pages, the next page still advances.
	if err := os.Remove(filepath.Join(dir, first.Data[1].Path)); err != nil {
		t.Fatal(err)
	}

	second, err := s.WorkspaceListFiles(context.Background(), protocol.ListFilesRequest{
		Recursive: true,
		PageQuery: protocol.PageQuery{Cursor: first.NextCursor, Limit: 2},
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Data) != 1 || second.Data[0].Path != "c.txt" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want c.txt and no cursor", second)
	}
	if _, err := s.WorkspaceListFiles(context.Background(), protocol.ListFilesRequest{
		Recursive: true,
		PageQuery: protocol.PageQuery{Limit: -1},
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("negative limit error = %v, want invalid_params", err)
	}
	if _, err := s.WorkspaceListFiles(context.Background(), protocol.ListFilesRequest{
		Recursive: true,
		PageQuery: protocol.PageQuery{Cursor: "!", Limit: 1},
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("invalid cursor error = %v, want invalid_params", err)
	}
}

func TestWorkspaceReadFileRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "leak.txt")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	s := newWorkspaceServer(root)

	if _, err := s.WorkspaceReadFile(context.Background(), protocol.ReadFileRequest{Path: "leak.txt"}); !errors.Is(err, protocol.ErrPathOutsideRoot) {
		t.Fatalf("read symlink escape err = %v, want ErrPathOutsideRoot", err)
	}
}

func TestWorkspaceReadFileWindowAndMaxBytes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "long.txt"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceServer(dir)

	got, err := s.WorkspaceReadFile(context.Background(), protocol.ReadFileRequest{Path: "f.txt", StartLine: 2, EndLine: 3})
	if err != nil {
		t.Fatalf("read window: %v", err)
	}
	if got.Content != "b\nc" || got.StartLine != 2 || got.EndLine != 3 || !got.Truncated {
		t.Fatalf("window = %+v, want lines 2..3 with truncated=true", got)
	}

	capped, err := s.WorkspaceReadFile(context.Background(), protocol.ReadFileRequest{Path: "long.txt", MaxBytes: 3})
	if err != nil {
		t.Fatalf("read capped: %v", err)
	}
	if capped.Content != "abc" || !capped.Truncated {
		t.Fatalf("capped = %+v, want abc with truncated=true", capped)
	}
}

func TestWorkspaceReadFileRejectsInvalidRange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceServer(dir)

	cases := []protocol.ReadFileRequest{
		{Path: "f.txt", StartLine: -1},
		{Path: "f.txt", EndLine: -1},
		{Path: "f.txt", MaxBytes: -1},
		{Path: "f.txt", EndLine: 2},
		{Path: "f.txt", StartLine: 3, EndLine: 2},
	}
	for _, tc := range cases {
		if _, err := s.WorkspaceReadFile(context.Background(), tc); !errors.Is(err, protocol.ErrInvalidParams) {
			t.Fatalf("WorkspaceReadFile(%+v) err = %v, want ErrInvalidParams", tc, err)
		}
	}
}

// TestWorkspaceGrep searches the workspace root, requires a query, and jails
// the optional sub-path. Depends on rg or grep being on PATH (skips if not).
func TestWorkspaceGrep(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.go": "package a\nfunc Needle() {}\n",
		"b.go": "package b\n// no match here\n",
		"c.go": "package c\nvar Needle = 1\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := newWorkspaceServer(dir)

	if _, err := s.WorkspaceGrep(context.Background(), protocol.GrepRequest{}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Errorf("empty query err = %v, want ErrInvalidParams", err)
	}

	got, err := s.WorkspaceGrep(context.Background(), protocol.GrepRequest{Query: "Needle"})
	if err != nil {
		t.Skipf("grep backend unavailable: %v", err) // no rg/grep on PATH
	}
	if got.Total != 2 || len(got.Matches) != 2 {
		t.Fatalf("grep Needle = %d matches / total %d, want 2/2", len(got.Matches), got.Total)
	}

	if _, err := s.WorkspaceGrep(context.Background(), protocol.GrepRequest{Query: "x", Path: "../out"}); !errors.Is(err, protocol.ErrPathOutsideRoot) {
		t.Errorf("escape path err = %v, want ErrPathOutsideRoot", err)
	}
}

type fakeSkillCatalog struct{ skills []skills.Info }

func (f fakeSkillCatalog) ListSkills(context.Context, string) ([]skills.Info, error) {
	return f.skills, nil
}

type fakeRecipeLister struct{ recipes []recipes.Recipe }

func (f fakeRecipeLister) List(context.Context, string) ([]recipes.Recipe, error) {
	return f.recipes, nil
}

// TestWorkspaceListSkills maps discovered skills onto the wire,
// carrying each one's scope through Source, and defaults cwd to the serve dir.
func TestWorkspaceListSkills(t *testing.T) {
	dir := t.TempDir()
	s := &Server{
		workspace: newWorkspaceCoordinator(dir, workspaceapp.Config{Skills: fakeSkillCatalog{skills: []skills.Info{
			{Name: "pdf", Description: "PDF tools", Scope: "project"},
			{Name: "web", Description: "web tools", Scope: "global"},
		}}}),
	}
	got, err := s.WorkspaceListSkills(context.Background(), protocol.WorkspaceListQuery{})
	if err != nil {
		t.Fatalf("listSkills: %v", err)
	}
	if len(got.Data) != 2 || got.Data[0].Name != "pdf" || got.Data[0].Source != "project" || got.Data[1].Source != "global" {
		t.Fatalf("skills = %+v, want pdf(project) + web(global)", got.Data)
	}
}

// TestWorkspaceListRecipes maps the runtime's discovered recipes onto the wire,
// carrying scope + body through, and defaults cwd to the serve dir.
func TestWorkspaceListRecipes(t *testing.T) {
	dir := t.TempDir()
	s := &Server{
		workspace: newWorkspaceCoordinator(dir, workspaceapp.Config{Recipes: fakeRecipeLister{recipes: []recipes.Recipe{
			{Name: "review", Description: "review diff", Body: "Review $ARGUMENTS", Scope: "project", Source: "/p/review.md"},
			{Name: "commit", Body: "Write a commit", Scope: "global", Source: "/g/commit.md"},
		}}}),
	}
	got, err := s.WorkspaceListRecipes(context.Background(), protocol.WorkspaceListQuery{})
	if err != nil {
		t.Fatalf("listRecipes: %v", err)
	}
	if len(got.Data) != 2 {
		t.Fatalf("recipes = %+v, want 2", got.Data)
	}
	if got.Data[0].Name != "review" || got.Data[0].Scope != "project" || got.Data[0].Body != "Review $ARGUMENTS" {
		t.Errorf("recipe[0] = %+v, want review(project) with body", got.Data[0])
	}
	if got.Data[1].Scope != "global" {
		t.Errorf("recipe[1].Scope = %q, want global", got.Data[1].Scope)
	}
}

// TestWorkspaceSubscribe: a watch-less subscribe receives the broadcast events
// (mcp/skills) and closes on ctx cancel. The watches path has its own coverage
// in filewatch_test.go.
func TestWorkspaceSubscribe(t *testing.T) {
	s := &Server{wsHub: newWorkspaceHub()}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, events, err := s.WorkspaceSubscribe(ctx, protocol.WorkspaceSubscribeRequest{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	s.PublishWorkspaceEvent(protocol.WorkspaceEvent{Type: "skills.changed"})
	select {
	case ev := <-events:
		if ev.Type != "skills.changed" {
			t.Fatalf("event = %+v, want skills.changed", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}

	cancel() // ctx done → unsubscribe closes the channel
	select {
	case _, ok := <-events:
		for ok { // drain any buffered, then expect close
			_, ok = <-events
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after ctx cancel")
	}
}

// TestWorkspaceSubscribeLifetimeIsTheRequest: a subscription's stream is bounded
// by its request ctx (client disconnect / the transport's forced shutdown), not
// by Server.Close — delivery owns no task group (§16 rule 5). Server.Close only
// gates NEW subscriptions; an in-flight, request-owned stream is left to its ctx.
func TestWorkspaceSubscribeLifetimeIsTheRequest(t *testing.T) {
	s := &Server{wsHub: newWorkspaceHub()}
	reqCtx, cancelReq := context.WithCancel(context.Background())
	_, events, err := s.WorkspaceSubscribe(reqCtx, protocol.WorkspaceSubscribeRequest{})
	if err != nil {
		t.Fatalf("WorkspaceSubscribe: %v", err)
	}

	// Server.Close gates new work but must not disturb an in-flight request-owned
	// stream (the transport joins active handlers on shutdown).
	s.Close()
	select {
	case _, ok := <-events:
		if !ok {
			t.Fatal("Server.Close must not close a request-owned stream")
		}
	case <-time.After(50 * time.Millisecond):
	}

	// The request ending is what closes the stream.
	cancelReq()
	select {
	case _, ok := <-events:
		for ok {
			_, ok = <-events
		}
	case <-time.After(time.Second):
		t.Fatal("stream not closed after request ctx cancel")
	}

	// A new subscription after Close is rejected.
	if _, _, err := s.WorkspaceSubscribe(context.Background(), protocol.WorkspaceSubscribeRequest{}); !errors.Is(err, errServerClosed) {
		t.Fatalf("subscribe after close err = %v, want errServerClosed", err)
	}
}

// TestAgentDocScope pins the cwd→home cascade classification.
func TestWorkspaceListAgentDocsRejectsUnavailableCwd(t *testing.T) {
	s := newWorkspaceServer(t.TempDir())
	missing := filepath.Join(t.TempDir(), "missing")

	_, err := s.WorkspaceListAgentDocs(context.Background(), protocol.WorkspaceListQuery{
		WorkspaceQuery: protocol.WorkspaceQuery{Cwd: missing},
	})
	if !errors.Is(err, protocol.ErrCwdUnavailable) {
		t.Fatalf("listAgentDocs err = %v, want ErrCwdUnavailable", err)
	}
}
