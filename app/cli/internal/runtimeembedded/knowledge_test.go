package runtimeembedded

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/knowledge"
)

type knowledgeBindingStub struct {
	t          *testing.T
	updates    []protocol.UpdateKnowledgeRequest
	updated    time.Time
	listed     *protocol.Page[protocol.KnowledgeEntry]
	nilList    bool
	dropUpdate bool
	getCalls   int
}

func (k *knowledgeBindingStub) ListKnowledge(_ context.Context, request protocol.WorkspaceQuery, options embedded.CallOptions) (*protocol.Page[protocol.KnowledgeEntry], error) {
	k.assertMeta(options.RequestMeta)
	if request.Workspace.Path != "/workspace" {
		k.t.Fatalf("list request = %+v", request)
	}
	if k.nilList {
		return nil, nil
	}
	if k.listed != nil {
		return k.listed, nil
	}
	return protocol.NewPage([]protocol.KnowledgeEntry{{
		Scope: protocol.KnowledgeScopeProjectRoot, Content: "project rules", Revision: "rev-project", UpdatedAt: k.updated,
	}}), nil
}

func TestKnowledgeAdapterRejectsUnaddressableCatalogs(t *testing.T) {
	now := time.Now()
	duplicate := protocol.KnowledgeEntry{Scope: protocol.KnowledgeScopeHome, Content: "prefs", Revision: "rev-home", UpdatedAt: now}
	for _, test := range []struct {
		name    string
		listed  *protocol.Page[protocol.KnowledgeEntry]
		nilList bool
	}{
		{name: "nil page", nilList: true},
		{name: "continuation without request cursor", listed: &protocol.Page[protocol.KnowledgeEntry]{NextCursor: "next"}},
		{name: "duplicate scope", listed: &protocol.Page[protocol.KnowledgeEntry]{Data: []protocol.KnowledgeEntry{duplicate, duplicate}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &knowledgeBindingStub{t: t, updated: now, listed: test.listed, nilList: test.nilList}
			adapter := &knowledgeAdapter{runtime: &Runtime{knowledge: stub, meta: requestMeta("test")}}
			if _, err := adapter.Entries(t.Context(), "/workspace"); err == nil {
				t.Fatal("unaddressable catalog was accepted")
			} else {
				requireRuntimeContractViolation(t, err)
			}
		})
	}
}

func (k *knowledgeBindingStub) GetKnowledge(_ context.Context, request protocol.GetKnowledgeRequest, options embedded.CallOptions) (*protocol.KnowledgeEntry, error) {
	k.assertMeta(options.RequestMeta)
	k.getCalls++
	if request.Scope == protocol.KnowledgeScopeHome {
		if request.Workspace != nil {
			k.t.Fatalf("home get leaked workspace: %+v", request)
		}
	} else if request.Workspace == nil || request.Workspace.Path != "/workspace" {
		k.t.Fatalf("workspace get request = %+v", request)
	}
	if !k.dropUpdate {
		for index := len(k.updates) - 1; index >= 0; index-- {
			update := k.updates[index]
			if update.Scope == request.Scope {
				return &protocol.KnowledgeEntry{
					Scope: request.Scope, Content: update.Content,
					Revision: "rev-updated", UpdatedAt: k.updated,
				}, nil
			}
		}
	}
	return &protocol.KnowledgeEntry{Scope: request.Scope, Content: "document", Revision: "rev-document"}, nil
}

func (k *knowledgeBindingStub) UpdateKnowledge(_ context.Context, request protocol.UpdateKnowledgeRequest, options embedded.CommandOptions) (*protocol.KnowledgeEntry, error) {
	k.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		k.t.Fatal("update has no idempotency key")
	}
	k.updates = append(k.updates, request)
	return &protocol.KnowledgeEntry{Scope: request.Scope, Content: request.Content, Revision: "rev-updated", UpdatedAt: k.updated}, nil
}

func (k *knowledgeBindingStub) assertMeta(meta protocol.RequestMeta) {
	k.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		k.t.Fatalf("request meta = %+v", meta)
	}
}

func TestKnowledgeAdapterKeepsCascadeScopeAndVerbatimContent(t *testing.T) {
	stub := &knowledgeBindingStub{t: t, updated: time.Now()}
	adapter := &knowledgeAdapter{runtime: &Runtime{knowledge: stub, meta: requestMeta("test")}}
	entries, err := adapter.Entries(t.Context(), "/workspace")
	if err != nil || len(entries) != 1 || entries[0].UpdatedAt == nil {
		t.Fatalf("Entries = (%+v, %v)", entries, err)
	}
	project, err := knowledge.NewTarget(knowledge.ProjectRoot, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := adapter.Document(t.Context(), project)
	if err != nil || entry.Scope != knowledge.ProjectRoot || entry.Content != "document" {
		t.Fatalf("Document = (%+v, %v)", entry, err)
	}
	home, err := knowledge.NewTarget(knowledge.Home, "")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := adapter.Save(t.Context(), knowledge.Update{
		Target: home, ExpectedRevision: "rev-home", Content: "line one\nline two\n",
	})
	if err != nil || updated.Revision != "rev-updated" {
		t.Fatal(err)
	}
	if len(stub.updates) != 1 || stub.updates[0].Workspace != nil || stub.updates[0].Content != "line one\nline two\n" ||
		stub.updates[0].ExpectedRevision != "rev-home" {
		t.Fatalf("updates = %+v", stub.updates)
	}
}

func TestKnowledgeAdapterDoesNotAcceptAnUpdateBeforeTheAuthoritativeReadConverges(t *testing.T) {
	stub := &knowledgeBindingStub{t: t, updated: time.Now(), dropUpdate: true}
	adapter := &knowledgeAdapter{runtime: &Runtime{knowledge: stub, meta: requestMeta("test")}}
	target, err := knowledge.NewTarget(knowledge.ProjectRoot, "/workspace")
	if err != nil {
		t.Fatal(err)
	}

	_, err = adapter.Save(t.Context(), knowledge.Update{
		Target: target, ExpectedRevision: "rev-document", Content: "replacement",
	})
	if err == nil {
		t.Fatal("unconverged knowledge update was accepted")
	}
	if stub.getCalls != 1 {
		t.Fatalf("authoritative reads = %d, want 1", stub.getCalls)
	}
}
