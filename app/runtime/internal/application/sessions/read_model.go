package sessions

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/pagination"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/session"
)

// Activity is the resolved durable Run activity used by Session read consumers.
// Running wins over Waiting when a Run tree contains both; Idle means the
// Session has no non-terminal Run. Admission and terminal maintenance are
// concurrency fences, not user-visible lifecycle facts.
type Activity string

const (
	ActivityRunning Activity = "running"
	ActivityWaiting Activity = "waiting"
	ActivityIdle    Activity = "idle"
)

// WorkspaceView is the live filesystem projection of a Session's exact
// Workspace identity.
type WorkspaceView struct {
	Path        string
	ProjectRoot string
	Missing     bool
}

// View is the complete application read model for a session. Live lineage and
// other aggregate-only state stay inside the session domain.
type View struct {
	ID              string
	Title           string
	Workspace       WorkspaceView
	Provider        string
	Model           string
	ReasoningEffort string
	Activity        Activity
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Favorite        bool
	Revision        uint64
}

// Activities resolves activity for the requested sessions in one durable read.
// Reading the Run projection instead of the process-local admission gate keeps
// sessions.list linearized with the runs.changed/sessions.changed notifications:
// once the terminal transaction publishes, a refetch cannot still report the
// already-terminal Run as running while maintenance releases its admission.
func (c *Coordinator) Activities(ctx context.Context, sessionIDs []string) (map[string]Activity, error) {
	activities := make(map[string]Activity, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return activities, nil
	}
	requested := make(map[string]struct{}, len(sessionIDs))
	for _, id := range sessionIDs {
		requested[id] = struct{}{}
		activities[id] = ActivityIdle
	}
	activeRuns, err := c.runs.ListNonTerminalRuns(ctx)
	if err != nil {
		return nil, err
	}
	for _, activeRun := range activeRuns {
		sessionID := activeRun.SessionID()
		if _, ok := requested[sessionID]; !ok {
			continue
		}
		switch activeRun.State().Status() {
		case run.StatusRunning:
			activities[sessionID] = ActivityRunning
		case run.StatusWaiting:
			if activities[sessionID] == ActivityIdle {
				activities[sessionID] = ActivityWaiting
			}
		case run.StatusFinished:
			return nil, fmt.Errorf(
				"sessions: non-terminal Run read returned finished Run %q",
				activeRun.ID(),
			)
		}
	}
	return activities, nil
}

// viewPageNamespace binds cursors to this session read independently of other
// paged reads.
const viewPageNamespace = "sessions"

// viewPageLimit is the widest session page this read will serve.
const viewPageLimit = 100

// ListViewPage resolves one page of user-facing sessions, continuing after
// cursor. The page is bounded by the query, so only the sessions being returned
// are resolved against the filesystem and the live-run registry.
func (c *Coordinator) ListViewPage(ctx context.Context, cursor string, limit int) (pagination.Page[View], error) {
	anchor, err := pagination.Decode(cursor, viewPageNamespace, nil)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	var afterFavorite bool
	var afterUpdatedAt int64
	var afterID string
	if len(anchor) > 0 {
		if len(anchor) != 3 {
			return pagination.Page[View]{}, pagination.ErrInvalidCursor
		}
		afterFavorite = anchor[0] == "1"
		if afterUpdatedAt, err = strconv.ParseInt(anchor[1], 10, 64); err != nil {
			return pagination.Page[View]{}, pagination.ErrInvalidCursor
		}
		afterID = anchor[2]
	}
	size, err := pagination.Limit(limit, viewPageLimit)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	values, err := c.sessions.ListPage(ctx, afterFavorite, afterUpdatedAt, afterID, size+1)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	bounded := pagination.PageOf(values, size, viewPageNamespace, nil, func(value session.Session) []string {
		favorite := "0"
		if value.Favorite() {
			favorite = "1"
		}
		return []string{favorite, strconv.FormatInt(value.UpdatedAt().UnixNano(), 10), value.ID()}
	})
	views, err := c.views(ctx, bounded.Rows)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	return pagination.Page[View]{Rows: views, NextCursor: bounded.NextCursor}, nil
}

// View resolves one session's complete application read model.
func (c *Coordinator) View(ctx context.Context, id string) (View, error) {
	value, err := c.sessions.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	views, err := c.views(ctx, []session.Session{value})
	if err != nil {
		return View{}, err
	}
	return views[0], nil
}

// CreateView admits a fresh session and returns its fully resolved read model.
func (c *Coordinator) CreateView(ctx context.Context, title, cwd string) (View, error) {
	value, err := c.Create(ctx, title, cwd)
	if err != nil {
		return View{}, err
	}
	return c.view(value, ActivityIdle)
}

// UpdateView applies an edit and returns its fully resolved read model.
func (c *Coordinator) UpdateView(ctx context.Context, id string, patch Patch) (View, error) {
	value, err := c.Update(ctx, id, patch)
	if err != nil {
		return View{}, err
	}
	activities, err := c.Activities(ctx, []string{value.ID()})
	if err != nil {
		return View{}, err
	}
	return c.view(value, activities[value.ID()])
}

// ForkView branches a session and returns the child session's fully resolved
// read model.
func (c *Coordinator) ForkView(ctx context.Context, spec ForkSpec) (View, error) {
	value, err := c.Fork(ctx, spec)
	if err != nil {
		return View{}, err
	}
	return c.view(value, ActivityIdle)
}

func (c *Coordinator) views(ctx context.Context, values []session.Session) ([]View, error) {
	ids := make([]string, len(values))
	for index, value := range values {
		ids[index] = value.ID()
	}
	activities, err := c.Activities(ctx, ids)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(values))
	for _, value := range values {
		view, err := c.view(value, activities[value.ID()])
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (c *Coordinator) view(value session.Session, activity Activity) (View, error) {
	workspace, err := c.paths.Inspect(value.Workspace().Path())
	if err != nil {
		return View{}, fmt.Errorf("sessions: inspect workspace %q: %w", value.Workspace().Path(), err)
	}
	selection := value.Selection()
	return View{
		ID:    value.ID(),
		Title: value.Title(),
		Workspace: WorkspaceView{
			Path: workspace.Path, ProjectRoot: workspace.ProjectRoot, Missing: workspace.Missing,
		},
		Provider:        selection.Provider(),
		Model:           selection.Model(),
		ReasoningEffort: selection.ReasoningEffort(),
		Activity:        activity,
		CreatedAt:       value.StartedAt(), UpdatedAt: value.UpdatedAt(),
		Favorite: value.Favorite(), Revision: value.Revision(),
	}, nil
}
