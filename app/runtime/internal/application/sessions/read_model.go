package sessions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/pagination"
)

// Activity is the resolved session activity view used by read consumers.
// Running is process-local admission state; Waiting is a durable open HITL
// interrupt; Idle means neither. This precedence is application policy.
type Activity string

const (
	ActivityRunning Activity = "running"
	ActivityWaiting Activity = "waiting"
	ActivityIdle    Activity = "idle"
)

// View is the complete application read model for a session. Live lineage and
// other aggregate-only state stay inside the session domain.
type View struct {
	ID          string
	Title       string
	CWD         string
	ProjectRoot string
	CWDMissing  bool
	Model       string
	Activity    Activity
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Favorite    bool
	Revision    uint64
}

// Activities resolves activity for the requested sessions in one use-case
// read. It centralizes the precedence between a live execution and a durable
// interrupt so every caller observes the same resolved state.
func (c *Coordinator) Activities(ctx context.Context, sessionIDs []string) (map[string]Activity, error) {
	activities := make(map[string]Activity, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return activities, nil
	}
	if c.admissions == nil {
		return nil, errors.New("sessions: admission gate is unavailable")
	}
	active := c.admissions.ActiveSessions()
	hasIdle := false
	for _, id := range sessionIDs {
		if active[id] {
			activities[id] = ActivityRunning
		} else {
			activities[id] = ActivityIdle
			hasIdle = true
		}
	}
	if !hasIdle || c.interrupts == nil {
		return activities, nil
	}
	filter := ""
	if len(sessionIDs) == 1 {
		filter = sessionIDs[0]
	}
	pending, err := c.interrupts.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, interrupt := range pending {
		if activities[interrupt.SessionID] == ActivityIdle {
			activities[interrupt.SessionID] = ActivityWaiting
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
	if c.sessions == nil {
		return pagination.Page[View]{}, errors.New("sessions: session store is unavailable")
	}
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
		if value.Favorite {
			favorite = "1"
		}
		return []string{favorite, strconv.FormatInt(value.UpdatedAt.UnixNano(), 10), value.ID}
	})
	views, err := c.views(ctx, bounded.Rows)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	return pagination.Page[View]{Rows: views, NextCursor: bounded.NextCursor}, nil
}

// View resolves one session's complete application read model.
func (c *Coordinator) View(ctx context.Context, id string) (View, error) {
	if c.sessions == nil {
		return View{}, errors.New("sessions: session store is unavailable")
	}
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
func (c *Coordinator) UpdateView(ctx context.Context, id string, patch session.Patch) (View, error) {
	value, err := c.Update(ctx, id, patch)
	if err != nil {
		return View{}, err
	}
	activities, err := c.Activities(ctx, []string{value.ID})
	if err != nil {
		return View{}, err
	}
	return c.view(value, activities[value.ID])
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
		ids[index] = value.ID
	}
	activities, err := c.Activities(ctx, ids)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(values))
	for _, value := range values {
		view, err := c.view(value, activities[value.ID])
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (c *Coordinator) view(value session.Session, activity Activity) (View, error) {
	if c.paths == nil {
		return View{}, errors.New("sessions: workspace inspector is unavailable")
	}
	workspace, err := c.paths.Inspect(value.CWD)
	if err != nil {
		return View{}, fmt.Errorf("sessions: inspect workspace %q: %w", value.CWD, err)
	}
	model := value.Model
	if model == "" {
		model = c.defaultModel
	}
	return View{
		ID:          value.ID,
		Title:       value.Title,
		CWD:         workspace.CWD,
		ProjectRoot: workspace.ProjectRoot,
		CWDMissing:  workspace.Missing,
		Model:       model,
		Activity:    activity,
		CreatedAt:   value.StartedAt,
		UpdatedAt:   value.UpdatedAt,
		Favorite:    value.Favorite,
		Revision:    value.Revision,
	}, nil
}
