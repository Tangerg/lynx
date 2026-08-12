package mock

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (r *Runtime) GetRun(ctx context.Context, runID string) (agent.Run, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.Run{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[runID]
	if run == nil {
		return agent.Run{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, runID)
	}
	return projectRun(run), nil
}

func (r *Runtime) ListRuns(ctx context.Context, query agent.RunQuery) (agent.RunPage, error) {
	if err := query.Validate(); err != nil {
		return agent.RunPage{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return agent.RunPage{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]agent.Run, 0, len(r.runOrder))
	for _, runID := range slices.Backward(r.runOrder) {
		run := r.runs[runID]
		if run == nil || (query.SessionID != "" && run.sessionID != query.SessionID) {
			continue
		}
		if !query.IncludeDescendants && !run.lineage.IsRoot() {
			continue
		}
		if len(query.Statuses) != 0 && !slices.Contains(query.Statuses, run.status) {
			continue
		}
		items = append(items, projectRun(run))
	}

	offset, err := pageOffset("run", query.Cursor, len(items))
	if err != nil {
		return agent.RunPage{}, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = defaultPageSize
	}
	limit = min(limit, maxPageSize)
	end := min(offset+limit, len(items))
	page := agent.RunPage{Items: slices.Clone(items[offset:end])}
	if end < len(items) {
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}
