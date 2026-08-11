package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/codebase"
)

type codebaseBinding interface {
	SearchCodebase(context.Context, protocol.CodebaseSearchRequest, embedded.CallOptions) (*protocol.CodebaseSearchResult, error)
	GetCodebaseStatus(context.Context, protocol.CodebaseStatusRequest, embedded.CallOptions) (*protocol.CodebaseStatus, error)
	ReindexCodebase(context.Context, protocol.CodebaseReindexRequest, embedded.CommandOptions) (*protocol.CodebaseReindexResponse, error)
}

type codebaseAdapter struct{ runtime *Runtime }

var _ codebase.Service = (*codebaseAdapter)(nil)

func (adapter *codebaseAdapter) Status(ctx context.Context, workspace string) (codebase.Status, error) {
	r := adapter.runtime
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return codebase.Status{}, errors.New("get codebase status: workspace is empty")
	}
	result, err := r.codebase.GetCodebaseStatus(ctx, protocol.CodebaseStatusRequest{
		Workspace: protocol.WorkspaceRef{Path: workspace},
	}, r.callOptions())
	if err != nil {
		return codebase.Status{}, classifyError(err)
	}
	return projectCodebaseStatus("get codebase status", result)
}

func (adapter *codebaseAdapter) Search(ctx context.Context, query codebase.Query) ([]codebase.Hit, error) {
	r := adapter.runtime
	if err := query.Validate(); err != nil {
		return nil, err
	}
	result, err := r.codebase.SearchCodebase(ctx, protocol.CodebaseSearchRequest{
		Workspace: protocol.WorkspaceRef{Path: query.Workspace}, Query: query.Text, Limit: query.Limit,
	}, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, errors.New("search codebase: runtime returned nil")
	}
	hits := make([]codebase.Hit, 0, len(result.Hits))
	for index, value := range result.Hits {
		hit := codebase.Hit{
			Path: value.Path, StartLine: value.StartLine, EndLine: value.EndLine,
			Snippet: value.Snippet, Score: value.Score,
		}
		if err := hit.Validate(); err != nil {
			return nil, fmt.Errorf("search codebase hit %d: %w", index+1, err)
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

func (adapter *codebaseAdapter) Reindex(ctx context.Context, workspace string) (codebase.ReindexOperation, error) {
	r := adapter.runtime
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return codebase.ReindexOperation{}, errors.New("reindex codebase: workspace is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return codebase.ReindexOperation{}, err
	}
	result, err := r.codebase.ReindexCodebase(ctx, protocol.CodebaseReindexRequest{
		Workspace: protocol.WorkspaceRef{Path: workspace},
	}, options)
	if err != nil {
		return codebase.ReindexOperation{}, classifyError(err)
	}
	if result == nil {
		return codebase.ReindexOperation{}, errors.New("reindex codebase: runtime returned nil")
	}
	operation := codebase.ReindexOperation{ID: result.OperationID}
	if err := operation.Validate(); err != nil {
		return codebase.ReindexOperation{}, fmt.Errorf("reindex codebase: %w", err)
	}
	return operation, nil
}

func projectCodebaseStatus(operation string, value *protocol.CodebaseStatus) (codebase.Status, error) {
	if value == nil {
		return codebase.Status{}, fmt.Errorf("%s: runtime returned nil", operation)
	}
	status := codebase.Status{
		State: codebase.State(value.State), ModelID: value.ModelID,
		FileCount: value.FileCount, ChunkCount: value.ChunkCount,
		Truncated: value.Truncated, OperationID: value.OperationID,
	}
	if value.IndexedAt != "" {
		indexedAt, err := time.Parse(time.RFC3339, value.IndexedAt)
		if err != nil {
			return codebase.Status{}, fmt.Errorf("%s: parse index time: %w", operation, err)
		}
		status.IndexedAt = &indexedAt
	}
	if err := status.Validate(); err != nil {
		return codebase.Status{}, fmt.Errorf("%s: %w", operation, err)
	}
	return status, nil
}
