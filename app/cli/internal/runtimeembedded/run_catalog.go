package runtimeembedded

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type runCatalogBinding interface {
	GetRun(context.Context, protocol.GetRunRequest, embedded.CallOptions) (*protocol.RunRef, error)
	ListRuns(context.Context, protocol.ListRunsRequest, embedded.CallOptions) (*protocol.Page[protocol.RunRef], error)
}

const (
	defaultRunPageSize = 20
	maximumRunPageSize = 100
)

func (r *Runtime) GetRun(ctx context.Context, runID string) (agent.Run, error) {
	if strings.TrimSpace(runID) == "" {
		return agent.Run{}, errors.New("get run: run id is empty")
	}
	value, err := r.runCatalog.GetRun(ctx, protocol.GetRunRequest{RunID: runID}, r.callOptions())
	if err != nil {
		return agent.Run{}, classifyError(err)
	}
	if value == nil {
		return agent.Run{}, runtimeContractViolation("get run returned nil")
	}
	projected, err := projectRun(*value)
	if err != nil {
		return agent.Run{}, runtimeContractViolation("get run returned an invalid run: %v", err)
	}
	if projected.ID != runID {
		return agent.Run{}, runtimeContractViolation("get run returned id %q for %q", projected.ID, runID)
	}
	return projected, nil
}

func (r *Runtime) ListRuns(ctx context.Context, query agent.RunQuery) (agent.RunPage, error) {
	if err := query.Validate(); err != nil {
		return agent.RunPage{}, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = defaultRunPageSize
	}
	limit = min(limit, maximumRunPageSize)
	var statuses []protocol.RunStatus
	if len(query.Statuses) != 0 {
		statuses = make([]protocol.RunStatus, len(query.Statuses))
		for index, status := range query.Statuses {
			statuses[index] = protocol.RunStatus(status)
		}
	}
	page, err := r.runCatalog.ListRuns(ctx, protocol.ListRunsRequest{
		SessionID: query.SessionID, Statuses: statuses, IncludeDescendants: query.IncludeDescendants,
		PageQuery: protocol.PageQuery{Cursor: query.Cursor, Limit: limit},
	}, r.callOptions())
	if err != nil {
		return agent.RunPage{}, classifyError(err)
	}
	return projectRunPage(page, query, limit)
}

func projectRunPage(page *protocol.Page[protocol.RunRef], query agent.RunQuery, limit int) (agent.RunPage, error) {
	if page == nil {
		return agent.RunPage{}, runtimeContractViolation("list runs returned a nil page")
	}
	if len(page.Data) > limit {
		return agent.RunPage{}, runtimeContractViolation("list runs returned %d rows for limit %d", len(page.Data), limit)
	}
	if page.NextCursor != "" && page.NextCursor == query.Cursor {
		return agent.RunPage{}, runtimeContractViolation("list runs returned its request cursor as the continuation")
	}
	projected := agent.RunPage{Items: make([]agent.Run, 0, len(page.Data)), NextCursor: page.NextCursor}
	for _, value := range page.Data {
		run, err := projectRun(value)
		if err != nil {
			return agent.RunPage{}, runtimeContractViolation("list runs returned an invalid run: %v", err)
		}
		if query.SessionID != "" && run.SessionID != query.SessionID {
			return agent.RunPage{}, runtimeContractViolation("list runs for session %q returned run %q from %q", query.SessionID, run.ID, run.SessionID)
		}
		if len(query.Statuses) != 0 && !slices.Contains(query.Statuses, run.Status) {
			return agent.RunPage{}, runtimeContractViolation("list runs returned run %q with unrequested status %q", run.ID, run.Status)
		}
		if !query.IncludeDescendants && !run.Lineage.IsRoot() {
			return agent.RunPage{}, runtimeContractViolation("list root runs returned child %q", run.ID)
		}
		projected.Items = append(projected.Items, run)
	}
	if err := projected.Validate(); err != nil {
		return agent.RunPage{}, runtimeContractViolation("list runs returned an invalid projection: %v", err)
	}
	return projected, nil
}
