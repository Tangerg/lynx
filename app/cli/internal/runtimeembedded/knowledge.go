package runtimeembedded

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/knowledge"
)

type knowledgeBinding interface {
	ListKnowledge(context.Context, protocol.WorkspaceQuery, embedded.CallOptions) (*protocol.Page[protocol.KnowledgeEntry], error)
	GetKnowledge(context.Context, protocol.GetKnowledgeRequest, embedded.CallOptions) (*protocol.KnowledgeEntry, error)
	UpdateKnowledge(context.Context, protocol.UpdateKnowledgeRequest, embedded.CommandOptions) (*protocol.KnowledgeEntry, error)
}

type knowledgeAdapter struct{ runtime *Runtime }

var _ knowledge.Service = (*knowledgeAdapter)(nil)

func (k *knowledgeAdapter) Entries(ctx context.Context, workspace string) ([]knowledge.Entry, error) {
	r := k.runtime
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, errors.New("list knowledge: workspace is empty")
	}
	page, err := r.knowledge.ListKnowledge(ctx, protocol.WorkspaceQuery{
		Workspace: protocol.WorkspaceRef{Path: workspace},
	}, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list knowledge", page)
	if err != nil {
		return nil, err
	}
	entries := make([]knowledge.Entry, 0, len(values))
	seen := make(map[knowledge.Scope]struct{}, len(values))
	for index, value := range values {
		entry := projectKnowledgeEntry(value)
		if err := entry.Validate(); err != nil {
			return nil, runtimeContractViolation("list knowledge item %d is invalid: %v", index+1, err)
		}
		if _, duplicate := seen[entry.Scope]; duplicate {
			return nil, runtimeContractViolation("list knowledge repeats %s scope", entry.Scope)
		}
		seen[entry.Scope] = struct{}{}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (k *knowledgeAdapter) Document(ctx context.Context, target knowledge.Target) (knowledge.Entry, error) {
	r := k.runtime
	if err := target.Validate(); err != nil {
		return knowledge.Entry{}, err
	}
	request := protocol.GetKnowledgeRequest{Scope: protocol.KnowledgeScope(target.Scope)}
	if target.Scope != knowledge.Home {
		request.Workspace = &protocol.WorkspaceRef{Path: target.Workspace}
	}
	result, err := r.knowledge.GetKnowledge(ctx, request, r.callOptions())
	if err != nil {
		return knowledge.Entry{}, classifyError(err)
	}
	if result == nil {
		return knowledge.Entry{}, runtimeContractViolation("get knowledge returned nil")
	}
	entry := projectKnowledgeEntry(*result)
	if err := entry.Validate(); err != nil {
		return knowledge.Entry{}, runtimeContractViolation("get knowledge returned an invalid entry: %v", err)
	}
	if entry.Scope != target.Scope {
		return knowledge.Entry{}, runtimeContractViolation("get knowledge returned %s scope, want %s", entry.Scope, target.Scope)
	}
	return entry, nil
}

func (k *knowledgeAdapter) Save(ctx context.Context, update knowledge.Update) (knowledge.Entry, error) {
	r := k.runtime
	if err := update.Validate(); err != nil {
		return knowledge.Entry{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return knowledge.Entry{}, err
	}
	target := update.Target
	request := protocol.UpdateKnowledgeRequest{
		Scope: protocol.KnowledgeScope(target.Scope), ExpectedRevision: update.ExpectedRevision, Content: update.Content,
	}
	if target.Scope != knowledge.Home {
		request.Workspace = &protocol.WorkspaceRef{Path: target.Workspace}
	}
	updated, err := r.knowledge.UpdateKnowledge(ctx, request, options)
	if err != nil {
		return knowledge.Entry{}, classifyError(err)
	}
	if updated == nil {
		return knowledge.Entry{}, runtimeContractViolation("update knowledge returned nil")
	}
	entry := projectKnowledgeEntry(*updated)
	if err := entry.Validate(); err != nil {
		return knowledge.Entry{}, runtimeContractViolation("update knowledge returned an invalid entry: %v", err)
	}
	if entry.Scope != target.Scope || entry.Content != update.Content {
		return knowledge.Entry{}, runtimeContractViolation("update knowledge returned a mismatched entry")
	}
	authoritative, err := k.Document(ctx, target)
	if err != nil {
		return knowledge.Entry{}, err
	}
	if authoritative.Content != update.Content {
		return knowledge.Entry{}, errors.New("verify knowledge update: authoritative document did not converge")
	}
	return authoritative, nil
}

func projectKnowledgeEntry(value protocol.KnowledgeEntry) knowledge.Entry {
	entry := knowledge.Entry{Scope: knowledge.Scope(value.Scope), Content: value.Content, Revision: value.Revision}
	if !value.UpdatedAt.IsZero() {
		entry.UpdatedAt = new(value.UpdatedAt)
	}
	return entry
}
