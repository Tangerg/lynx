package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/knowledge"
)

type knowledgeBinding interface {
	ListKnowledge(context.Context, protocol.WorkspaceQuery, embedded.CallOptions) (*protocol.Page[protocol.KnowledgeEntry], error)
	GetKnowledge(context.Context, protocol.GetKnowledgeRequest, embedded.CallOptions) (*protocol.KnowledgeEntry, error)
	UpdateKnowledge(context.Context, protocol.UpdateKnowledgeRequest, embedded.CommandOptions) error
}

type knowledgeAdapter struct{ runtime *Runtime }

var _ knowledge.Service = (*knowledgeAdapter)(nil)

func (adapter *knowledgeAdapter) Entries(ctx context.Context, workspace string) ([]knowledge.Entry, error) {
	r := adapter.runtime
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
	if page == nil {
		return nil, errors.New("list knowledge: runtime returned nil")
	}
	if page.NextCursor != "" {
		return nil, errors.New("list knowledge: runtime returned an unusable continuation cursor")
	}
	entries := make([]knowledge.Entry, 0, len(page.Data))
	seen := make(map[knowledge.Scope]struct{}, len(page.Data))
	for index, value := range page.Data {
		entry := projectKnowledgeEntry(value)
		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("list knowledge item %d: %w", index+1, err)
		}
		if _, duplicate := seen[entry.Scope]; duplicate {
			return nil, fmt.Errorf("list knowledge repeats %s scope", entry.Scope)
		}
		seen[entry.Scope] = struct{}{}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (adapter *knowledgeAdapter) Document(ctx context.Context, target knowledge.Target) (knowledge.Entry, error) {
	r := adapter.runtime
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
		return knowledge.Entry{}, errors.New("get knowledge: runtime returned nil")
	}
	entry := projectKnowledgeEntry(*result)
	if err := entry.Validate(); err != nil {
		return knowledge.Entry{}, fmt.Errorf("get knowledge: %w", err)
	}
	if entry.Scope != target.Scope {
		return knowledge.Entry{}, fmt.Errorf("get knowledge returned %s scope, want %s", entry.Scope, target.Scope)
	}
	return entry, nil
}

func (adapter *knowledgeAdapter) Save(ctx context.Context, target knowledge.Target, content string) error {
	r := adapter.runtime
	if err := target.Validate(); err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	request := protocol.UpdateKnowledgeRequest{Scope: protocol.KnowledgeScope(target.Scope), Content: content}
	if target.Scope != knowledge.Home {
		request.Workspace = &protocol.WorkspaceRef{Path: target.Workspace}
	}
	return classifyError(r.knowledge.UpdateKnowledge(ctx, request, options))
}

func projectKnowledgeEntry(value protocol.KnowledgeEntry) knowledge.Entry {
	entry := knowledge.Entry{Scope: knowledge.Scope(value.Scope), Content: value.Content}
	if !value.UpdatedAt.IsZero() {
		entry.UpdatedAt = new(value.UpdatedAt)
	}
	return entry
}
