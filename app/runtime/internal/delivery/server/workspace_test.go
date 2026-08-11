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
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

type workspaceTestConfig struct {
	Knowledge workspaceapp.KnowledgeStore
	Skills    workspaceapp.SkillCatalog
	Curator   workspaceapp.SkillCurator
	Proposals workspaceapp.SkillProposals
	Hooks     workspaceapp.HookInspector
	Trust     workspaceapp.HookTrustStore
	Recipes   workspaceapp.RecipeLister
	Watcher   workspaceapp.GitStateWatcher
}

type workspaceSurfaces struct {
	roots     *workspaceapp.Scope
	files     *workspaceapp.Files
	vcs       *workspaceapp.VCS
	discovery *workspaceapp.Discovery
	knowledge *workspaceapp.Knowledge
	skills    *workspaceapp.Skills
	hooks     *workspaceapp.Hooks
	watch     *workspaceapp.GitWatch
}

func newWorkspaceSurfaces(cwd string, cfg workspaceTestConfig) workspaceSurfaces {
	roots := workspaceapp.NewScope(cwd, cwd, workspacepath.Resolver{})
	watcher := cfg.Watcher
	if watcher == nil {
		watcher = workspaceadapter.GitWatcher{}
	}
	return workspaceSurfaces{
		roots:     roots,
		files:     workspaceapp.NewFiles(roots, workspaceadapter.FileBrowser{}),
		vcs:       workspaceapp.NewVCS(roots, workspaceadapter.VCS{}),
		discovery: workspaceapp.NewDiscovery(roots, nil, nil, cfg.Recipes),
		knowledge: workspaceapp.NewKnowledge(roots, workspacepath.Resolver{}, cfg.Knowledge),
		skills:    workspaceapp.NewSkills(roots, cfg.Skills, cfg.Curator, cfg.Proposals, nil),
		hooks:     workspaceapp.NewHooks(roots, cfg.Hooks, cfg.Trust),
		watch:     workspaceapp.NewGitWatch(roots, watcher),
	}
}

func applyWorkspaceSurfaces(s *Server, surfaces workspaceSurfaces) {
	s.workspaceFiles = surfaces.files
	s.workspaceVCS = surfaces.vcs
	s.workspaceDiscovery = surfaces.discovery
	s.workspaceKnowledge = surfaces.knowledge
	s.workspaceSkills = surfaces.skills
	s.workspaceHooks = surfaces.hooks
	s.workspaceWatch = surfaces.watch
}

func newWorkspaceServer(cwd string) *Server {
	s := &Server{}
	applyWorkspaceSurfaces(s, newWorkspaceSurfaces(cwd, workspaceTestConfig{}))
	return s
}

func newWorkspaceServerWithConfig(cwd string, cfg workspaceTestConfig) *Server {
	s := &Server{}
	applyWorkspaceSurfaces(s, newWorkspaceSurfaces(cwd, cfg))
	return s
}

// TestWorkspaceGetFileHead reads the first N lines of a cwd-relative file,
// numbers them 1-based, and refuses a path that climbs out of the root.
func TestWorkspaceGetFileHead(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceServer(dir)

	got, err := s.GetWorkspaceFileHead(context.Background(), protocol.GetFileHeadRequest{Path: "f.txt", Lines: 2})
	if err != nil {
		t.Fatalf("getFileHead: %v", err)
	}
	if len(got.Lines) != 2 || got.Lines[0].LineNumber != 1 || got.Lines[0].Text != "a" || got.Lines[1].LineNumber != 2 {
		t.Fatalf("lines = %+v, want first two lines numbered 1,2", got.Lines)
	}

	if _, err := s.GetWorkspaceFileHead(context.Background(), protocol.GetFileHeadRequest{Path: "../escape"}); !errors.Is(err, protocol.ErrPathOutsideRoot) {
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

	first, err := s.ListWorkspaceFiles(context.Background(), protocol.ListFilesRequest{Recursive: true, PageQuery: protocol.PageQuery{Limit: 2}})
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

	second, err := s.ListWorkspaceFiles(context.Background(), protocol.ListFilesRequest{
		Recursive: true,
		PageQuery: protocol.PageQuery{Cursor: first.NextCursor, Limit: 2},
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Data) != 1 || second.Data[0].Path != "c.txt" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want c.txt and no cursor", second)
	}
	if _, err := s.ListWorkspaceFiles(context.Background(), protocol.ListFilesRequest{
		Recursive: true,
		PageQuery: protocol.PageQuery{Limit: -1},
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("negative limit error = %v, want invalid_params", err)
	}
	if _, err := s.ListWorkspaceFiles(context.Background(), protocol.ListFilesRequest{
		Recursive: true,
		PageQuery: protocol.PageQuery{Cursor: "!", Limit: 1},
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("invalid cursor error = %v, want invalid_params", err)
	}
	if _, err := s.ListWorkspaceFiles(context.Background(), protocol.ListFilesRequest{
		Recursive: false,
		PageQuery: protocol.PageQuery{Cursor: first.NextCursor, Limit: 1},
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("cross-query cursor error = %v, want invalid_params", err)
	}
	if _, err := s.ListWorkspaceFiles(context.Background(), protocol.ListFilesRequest{
		Glob: "[",
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("invalid glob error = %v, want invalid_params", err)
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

	if _, err := s.ReadWorkspaceFile(context.Background(), protocol.ReadFileRequest{Path: "leak.txt"}); !errors.Is(err, protocol.ErrPathOutsideRoot) {
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

	got, err := s.ReadWorkspaceFile(context.Background(), protocol.ReadFileRequest{Path: "f.txt", StartLine: 2, EndLine: 3})
	if err != nil {
		t.Fatalf("read window: %v", err)
	}
	if got.Content != "b\nc" || got.StartLine != 2 || got.EndLine != 3 || !got.Truncated {
		t.Fatalf("window = %+v, want lines 2..3 with truncated=true", got)
	}

	capped, err := s.ReadWorkspaceFile(context.Background(), protocol.ReadFileRequest{Path: "long.txt", MaxBytes: 3})
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
		if _, err := s.ReadWorkspaceFile(context.Background(), tc); !errors.Is(err, protocol.ErrInvalidParams) {
			t.Fatalf("ReadWorkspaceFile(%+v) err = %v, want ErrInvalidParams", tc, err)
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

	if _, err := s.GrepWorkspace(context.Background(), protocol.GrepRequest{}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Errorf("empty query err = %v, want ErrInvalidParams", err)
	}

	got, err := s.GrepWorkspace(context.Background(), protocol.GrepRequest{Query: "Needle"})
	if err != nil {
		t.Skipf("grep backend unavailable: %v", err) // no rg/grep on PATH
	}
	if got.Total != 2 || len(got.Matches) != 2 {
		t.Fatalf("grep Needle = %d matches / total %d, want 2/2", len(got.Matches), got.Total)
	}

	if _, err := s.GrepWorkspace(context.Background(), protocol.GrepRequest{Query: "x", Path: "../out"}); !errors.Is(err, protocol.ErrPathOutsideRoot) {
		t.Errorf("escape path err = %v, want ErrPathOutsideRoot", err)
	}
}

type fakeSkillCatalog struct{ skills []workspaceapp.SkillSummary }

func (f fakeSkillCatalog) List(context.Context, string) ([]workspaceapp.SkillSummary, error) {
	return f.skills, nil
}

type fakeRecipeLister struct{ recipes []workspaceapp.Recipe }

func (f fakeRecipeLister) List(context.Context, string) ([]workspaceapp.Recipe, error) {
	return f.recipes, nil
}

// TestListDiscoveredSkills maps discovered skills onto the wire,
// carrying each one's scope through the wire, and defaults cwd to the serve dir.
func TestListDiscoveredSkills(t *testing.T) {
	dir := t.TempDir()
	s := newWorkspaceServerWithConfig(dir, workspaceTestConfig{Skills: fakeSkillCatalog{skills: []workspaceapp.SkillSummary{
		{Name: "pdf", Description: "PDF tools", Scope: "project"},
		{Name: "web", Description: "web tools", Scope: "user"},
	}}})
	got, err := s.ListDiscoveredSkills(context.Background(), protocol.WorkspaceQuery{})
	if err != nil {
		t.Fatalf("listSkills: %v", err)
	}
	if len(got.Data) != 2 || got.Data[0].Name != "pdf" || got.Data[0].Scope != "project" || got.Data[1].Scope != "user" {
		t.Fatalf("skills = %+v, want pdf(project) + web(user)", got.Data)
	}
}

// TestListRecipes maps the runtime's discovered recipes onto the wire,
// carrying scope + body through, and defaults cwd to the serve dir.
func TestListRecipes(t *testing.T) {
	dir := t.TempDir()
	s := newWorkspaceServerWithConfig(dir, workspaceTestConfig{Recipes: fakeRecipeLister{recipes: []workspaceapp.Recipe{
		{Name: "review", Description: "review diff", Body: "Review $ARGUMENTS", Scope: workspaceapp.RecipeScopeProject, Source: "/p/review.md"},
		{Name: "commit", Body: "Write a commit", Scope: workspaceapp.RecipeScopeGlobal, Source: "/g/commit.md"},
	}}})
	got, err := s.ListRecipes(context.Background(), protocol.WorkspaceQuery{})
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
	s := &Server{workspaceHub: newWorkspaceHub()}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	events := drainSeq(ctx, seq)
	s.workspaceHub.publish(protocol.RuntimeEvent{Type: "skills.changed"})
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

func TestWorkspaceSubscribe_EarlyRangeStopReleasesSubscription(t *testing.T) {
	s := &Server{workspaceHub: newWorkspaceHub()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	s.workspaceHub.publish(protocol.RuntimeEvent{Type: protocol.RuntimeSkillsChanged})
	for range seq {
		break
	}

	s.workspaceHub.mu.Lock()
	subscribers := len(s.workspaceHub.subscriptions)
	s.workspaceHub.mu.Unlock()
	if subscribers != 0 {
		t.Fatalf("subscriptions after range stop = %d, want 0", subscribers)
	}
}

// TestWorkspaceSubscribeLifetimeIsTheRequest: a subscription's stream is bounded
// by its request ctx (client disconnect / the transport's forced shutdown), not
// by Server.Close — delivery owns no task group (§16 rule 5). Server.Close only
// gates NEW subscriptions; an in-flight, request-owned stream is left to its ctx.
func TestWorkspaceSubscribeLifetimeIsTheRequest(t *testing.T) {
	s := &Server{workspaceHub: newWorkspaceHub()}
	reqCtx, cancelReq := context.WithCancel(context.Background())
	_, seq, err := s.SubscribeRuntime(reqCtx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	})
	if err != nil {
		t.Fatalf("SubscribeRuntime: %v", err)
	}
	events := drainSeq(reqCtx, seq)

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

	// A new subscription after Close is rejected. The request is a VALID one, so the
	// refusal is about the closed server rather than about the request.
	if _, _, err := s.SubscribeRuntime(context.Background(), protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	}); !errors.Is(err, errSubscriptionAdmissionsClosed) {
		t.Fatalf("subscribe after close err = %v, want errSubscriptionAdmissionsClosed", err)
	}
}

// TestAgentDocScope pins the cwd→home cascade classification.
func TestListAgentDocsRejectsUnavailableCWD(t *testing.T) {
	s := newWorkspaceServer(t.TempDir())
	missing := filepath.Join(t.TempDir(), "missing")

	_, err := s.ListAgentDocs(context.Background(), protocol.WorkspaceQuery{
		Workspace: protocol.WorkspaceRef{Path: missing},
	})
	if !errors.Is(err, protocol.ErrWorkspaceUnavailable) {
		t.Fatalf("listAgentDocs err = %v, want ErrWorkspaceUnavailable", err)
	}
}
