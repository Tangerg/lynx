package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// fakeKnowledgeStore is a workspace knowledge store recording the coordinator's
// calls, so the knowledge delivery handlers can be tested against a wired store
// (or, when nil, against the disabled path).
type fakeKnowledgeStore struct {
	entries         []knowledge.Entry
	listCWD         string
	listProjectRoot string
	getScope        knowledge.Scope
	getCWD          string
	getEntry        knowledge.Entry
	updateScope     knowledge.Scope
	updateCWD       string
	updateRevision  string
	updateContent   string
	updateErr       error
}

func (s *fakeKnowledgeStore) List(_ context.Context, cwd, projectRoot string) ([]knowledge.Entry, error) {
	s.listCWD = cwd
	s.listProjectRoot = projectRoot
	return s.entries, nil
}

func (s *fakeKnowledgeStore) Get(_ context.Context, scope knowledge.Scope, cwd string) (knowledge.Entry, error) {
	s.getScope = scope
	s.getCWD = cwd
	return s.getEntry, nil
}

func (s *fakeKnowledgeStore) Update(_ context.Context, scope knowledge.Scope, cwd, expectedRevision, content string) (knowledge.Entry, error) {
	s.updateScope = scope
	s.updateCWD = cwd
	s.updateRevision = expectedRevision
	s.updateContent = content
	if s.updateErr != nil {
		return knowledge.Entry{}, s.updateErr
	}
	return knowledge.Entry{Scope: scope, Content: content, Revision: "rev-2"}, nil
}

// serverWithKnowledge builds a test Server whose knowledge use case is backed by
// store (nil store means the capability is unavailable).
func serverWithKnowledge(store workspaceapp.KnowledgeStore) *Server {
	s := newTestServer(&stubRuntime{})
	applyWorkspaceSurfaces(s, newWorkspaceSurfaces("", workspaceTestConfig{Knowledge: store}))
	s.features.knowledge = store != nil
	return s
}

func TestListKnowledgeWithoutStoreReturnsCapabilityError(t *testing.T) {
	s := serverWithKnowledge(nil)

	_, err := s.ListKnowledge(context.Background(), protocol.WorkspaceQuery{
		Workspace: protocol.WorkspaceRef{Path: "/repo"},
	})
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("list knowledge err = %v, want capability_not_negotiated", err)
	}
}

func TestKnowledgeHandlersReturnCapabilityErrorWithoutStore(t *testing.T) {
	s := serverWithKnowledge(nil)

	_, err := s.GetKnowledge(context.Background(), protocol.GetKnowledgeRequest{Scope: protocol.KnowledgeScopeHome})
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("get knowledge err = %v, want capability_not_negotiated", err)
	}
	_, err = s.UpdateKnowledge(context.Background(), protocol.UpdateKnowledgeRequest{
		Scope: protocol.KnowledgeScopeHome, ExpectedRevision: "rev-1", Content: "prefs",
	})
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("update knowledge err = %v, want capability_not_negotiated", err)
	}
}

func TestListKnowledgeMapsEntriesToWire(t *testing.T) {
	captured := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	repo := t.TempDir()
	store := &fakeKnowledgeStore{
		entries: []knowledge.Entry{{
			Scope:     knowledge.ScopeHome,
			Content:   "Use short answers",
			Revision:  "rev-1",
			UpdatedAt: captured,
		}},
	}
	s := serverWithKnowledge(store)

	got, err := s.ListKnowledge(context.Background(), protocol.WorkspaceQuery{
		Workspace: protocol.WorkspaceRef{Path: repo},
	})
	if err != nil {
		t.Fatalf("list knowledge: %v", err)
	}
	if store.listCWD != canonicalWorkspacePath(t, repo) {
		t.Fatalf("cwd = %q, want %q", store.listCWD, canonicalWorkspacePath(t, repo))
	}
	if len(got.Data) != 1 || got.Data[0].Scope != protocol.KnowledgeScopeHome || got.Data[0].UpdatedAt != captured {
		t.Fatalf("wire knowledge = %+v", got.Data)
	}
}

func TestGetAndUpdateKnowledgeMapScopeToRuntime(t *testing.T) {
	store := &fakeKnowledgeStore{getEntry: knowledge.Entry{
		Scope: knowledge.ScopeProjectRoot, Content: "project notes", Revision: "rev-1",
	}}
	s := serverWithKnowledge(store)
	repo := t.TempDir()

	got, err := s.GetKnowledge(context.Background(), protocol.GetKnowledgeRequest{
		Scope: protocol.KnowledgeScopeProjectRoot, Workspace: &protocol.WorkspaceRef{Path: repo},
	})
	if err != nil {
		t.Fatalf("get knowledge: %v", err)
	}
	if got.Content != "project notes" || store.getScope != knowledge.ScopeProjectRoot || store.getCWD != canonicalWorkspacePath(t, repo) {
		t.Fatalf("get wire=%+v scope=%v cwd=%q", got, store.getScope, store.getCWD)
	}

	updated, err := s.UpdateKnowledge(context.Background(), protocol.UpdateKnowledgeRequest{
		Scope:            protocol.KnowledgeScopeHome,
		Workspace:        &protocol.WorkspaceRef{Path: "/ignored"},
		ExpectedRevision: "rev-1",
		Content:          "global prefs",
	})
	if err != nil {
		t.Fatalf("update knowledge: %v", err)
	}
	if updated.Revision != "rev-2" || store.updateScope != knowledge.ScopeHome || store.updateCWD != "" || store.updateRevision != "rev-1" || store.updateContent != "global prefs" {
		t.Fatalf("update scope=%v cwd=%q content=%q", store.updateScope, store.updateCWD, store.updateContent)
	}
}

func TestProjectKnowledgeRejectsUnavailableCWD(t *testing.T) {
	store := &fakeKnowledgeStore{}
	s := serverWithKnowledge(store)
	missing := filepath.Join(t.TempDir(), "missing")

	if _, err := s.GetKnowledge(context.Background(), protocol.GetKnowledgeRequest{
		Scope:     protocol.KnowledgeScopeCWD,
		Workspace: &protocol.WorkspaceRef{Path: missing},
	}); !errors.Is(err, protocol.ErrWorkspaceUnavailable) {
		t.Fatalf("get knowledge err = %v, want ErrWorkspaceUnavailable", err)
	}
	if _, err := s.UpdateKnowledge(context.Background(), protocol.UpdateKnowledgeRequest{
		Scope:            protocol.KnowledgeScopeProjectRoot,
		Workspace:        &protocol.WorkspaceRef{Path: missing},
		ExpectedRevision: "rev-1",
		Content:          "notes",
	}); !errors.Is(err, protocol.ErrWorkspaceUnavailable) {
		t.Fatalf("update knowledge err = %v, want ErrWorkspaceUnavailable", err)
	}
}

func TestUpdateKnowledgeMapsStaleRevisionToProtocolConflict(t *testing.T) {
	s := serverWithKnowledge(&fakeKnowledgeStore{updateErr: knowledge.ErrRevisionConflict})

	_, err := s.UpdateKnowledge(t.Context(), protocol.UpdateKnowledgeRequest{
		Scope: protocol.KnowledgeScopeHome, ExpectedRevision: "rev-old", Content: "stale",
	})
	if !errors.Is(err, protocol.ErrRevisionConflict) {
		t.Fatalf("UpdateKnowledge err = %v, want revision_conflict", err)
	}
}

func TestKnowledgeHandlersMapPhysicalPathEscape(t *testing.T) {
	s := serverWithKnowledge(&fakeKnowledgeStore{updateErr: knowledge.ErrPathOutsideScope})

	_, err := s.UpdateKnowledge(t.Context(), protocol.UpdateKnowledgeRequest{
		Scope: protocol.KnowledgeScopeHome, ExpectedRevision: "rev-1", Content: "outside",
	})
	if !errors.Is(err, protocol.ErrPathOutsideRoot) {
		t.Fatalf("UpdateKnowledge error = %v, want path_outside_root", err)
	}
}

func TestKnowledgeMappingRejectsUnknownScopes(t *testing.T) {
	s := serverWithKnowledge(&fakeKnowledgeStore{})

	if _, err := s.GetKnowledge(t.Context(), protocol.GetKnowledgeRequest{
		Scope: protocol.KnowledgeScope("workspace"),
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("GetKnowledge err = %v, want ErrInvalidParams", err)
	}
	if _, err := presentKnowledgeScope(knowledge.Scope("workspace")); err == nil {
		t.Fatal("presentKnowledgeScope accepted an unknown domain scope")
	}
}
