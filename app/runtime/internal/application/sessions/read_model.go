package sessions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// SessionState is the resolved session activity view used by read adapters.
// Running is process-local admission state; Waiting is a durable open HITL
// interrupt; Idle means neither. This precedence is application policy.
type SessionState string

const (
	SessionRunning SessionState = "running"
	SessionWaiting SessionState = "waiting"
	SessionIdle    SessionState = "idle"
)

// SessionView is the complete application read model for a user-facing
// session. It deliberately contains only values Delivery may project; live
// lineage and other aggregate-only state stay inside the session domain.
type SessionView struct {
	ID          string
	Title       string
	Cwd         string
	ProjectRoot string
	CwdMissing  bool
	Model       string
	State       SessionState
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Favorite    bool
	Revision    uint64
}

// SessionStates resolves activity for the requested sessions in one use-case
// read. It centralizes the precedence between a live turn and a durable
// interrupt so Delivery only projects the resolved state.
func (c *Coordinator) SessionStates(ctx context.Context, sessionIDs []string) (map[string]SessionState, error) {
	states := make(map[string]SessionState, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return states, nil
	}
	if c.admissions == nil {
		return nil, errors.New("sessions: admission gate is unavailable")
	}
	active := c.admissions.ActiveSessions()
	hasIdle := false
	for _, id := range sessionIDs {
		if active[id] {
			states[id] = SessionRunning
		} else {
			states[id] = SessionIdle
			hasIdle = true
		}
	}
	if !hasIdle || c.interrupts == nil {
		return states, nil
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
		if states[interrupt.SessionID] == SessionIdle {
			states[interrupt.SessionID] = SessionWaiting
		}
	}
	return states, nil
}

// ListViews resolves every user-facing session as one application read model.
// Callers that only want a page ask for one — resolving a view touches the
// filesystem and the live-run registry per session, so the whole list is a real
// cost, not a slice to be narrowed afterward.
func (c *Coordinator) ListViews(ctx context.Context) ([]SessionView, error) {
	if c.sessions == nil {
		return nil, errors.New("sessions: session store is unavailable")
	}
	values, err := c.sessions.List(ctx)
	if err != nil {
		return nil, err
	}
	return c.views(ctx, values)
}

// viewPageMethod names the query a page cursor belongs to, so a cursor minted by
// another read is rejected instead of continuing this one.
const viewPageMethod = "sessions.list"

// viewPageLimit is the widest sessions.list page this read will serve.
const viewPageLimit = 100

// ListViewPage resolves one page of user-facing sessions, continuing after
// cursor. The page is bounded by the query, so only the sessions being returned
// are resolved against the filesystem and the live-run registry.
func (c *Coordinator) ListViewPage(ctx context.Context, cursor string, limit int) (keyset.Page[SessionView], error) {
	if c.sessions == nil {
		return keyset.Page[SessionView]{}, errors.New("sessions: session store is unavailable")
	}
	anchor, err := keyset.Decode(cursor, viewPageMethod, nil)
	if err != nil {
		return keyset.Page[SessionView]{}, err
	}
	var afterFavorite bool
	var afterUpdatedAt int64
	var afterID string
	if len(anchor) > 0 {
		if len(anchor) != 3 {
			return keyset.Page[SessionView]{}, keyset.ErrInvalidCursor
		}
		afterFavorite = anchor[0] == "1"
		if afterUpdatedAt, err = strconv.ParseInt(anchor[1], 10, 64); err != nil {
			return keyset.Page[SessionView]{}, keyset.ErrInvalidCursor
		}
		afterID = anchor[2]
	}
	size, err := keyset.Limit(limit, viewPageLimit)
	if err != nil {
		return keyset.Page[SessionView]{}, err
	}
	values, err := c.sessions.ListPage(ctx, afterFavorite, afterUpdatedAt, afterID, size+1)
	if err != nil {
		return keyset.Page[SessionView]{}, err
	}
	bounded := keyset.PageOf(values, size, viewPageMethod, nil, func(value session.Session) []string {
		favorite := "0"
		if value.Favorite {
			favorite = "1"
		}
		return []string{favorite, strconv.FormatInt(value.UpdatedAt.UnixNano(), 10), value.ID}
	})
	views, err := c.views(ctx, bounded.Rows)
	if err != nil {
		return keyset.Page[SessionView]{}, err
	}
	return keyset.Page[SessionView]{Rows: views, NextCursor: bounded.NextCursor}, nil
}

// View resolves one session's complete application read model.
func (c *Coordinator) View(ctx context.Context, id string) (SessionView, error) {
	if c.sessions == nil {
		return SessionView{}, errors.New("sessions: session store is unavailable")
	}
	value, err := c.sessions.Get(ctx, id)
	if err != nil {
		return SessionView{}, err
	}
	views, err := c.views(ctx, []session.Session{value})
	if err != nil {
		return SessionView{}, err
	}
	return views[0], nil
}

// CreateView admits a fresh session and returns its fully resolved read model.
func (c *Coordinator) CreateView(ctx context.Context, title, cwd string) (SessionView, error) {
	value, err := c.Create(ctx, title, cwd)
	if err != nil {
		return SessionView{}, err
	}
	return c.view(ctx, value, SessionIdle)
}

// UpdateView applies an edit and returns its fully resolved read model.
func (c *Coordinator) UpdateView(ctx context.Context, id string, patch session.Patch) (SessionView, error) {
	value, err := c.Update(ctx, id, patch)
	if err != nil {
		return SessionView{}, err
	}
	states, err := c.SessionStates(ctx, []string{value.ID})
	if err != nil {
		return SessionView{}, err
	}
	return c.view(ctx, value, states[value.ID])
}

// ForkView branches a session and returns the child session's fully resolved
// read model.
func (c *Coordinator) ForkView(ctx context.Context, spec ForkSpec) (SessionView, error) {
	value, err := c.Fork(ctx, spec)
	if err != nil {
		return SessionView{}, err
	}
	return c.view(ctx, value, SessionIdle)
}

func (c *Coordinator) views(ctx context.Context, values []session.Session) ([]SessionView, error) {
	ids := make([]string, len(values))
	for index, value := range values {
		ids[index] = value.ID
	}
	states, err := c.SessionStates(ctx, ids)
	if err != nil {
		return nil, err
	}
	views := make([]SessionView, 0, len(values))
	for _, value := range values {
		view, err := c.view(ctx, value, states[value.ID])
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (c *Coordinator) view(ctx context.Context, value session.Session, state SessionState) (SessionView, error) {
	if c.paths == nil {
		return SessionView{}, errors.New("sessions: workspace inspector is unavailable")
	}
	workspace, err := c.paths.Inspect(value.Cwd)
	if err != nil {
		return SessionView{}, fmt.Errorf("sessions: inspect workspace %q: %w", value.Cwd, err)
	}
	model := value.Model
	if model == "" {
		model = c.defaultModel
	}
	return SessionView{
		ID:          value.ID,
		Title:       value.Title,
		Cwd:         workspace.Cwd,
		ProjectRoot: workspace.ProjectRoot,
		CwdMissing:  workspace.Missing,
		Model:       model,
		State:       state,
		CreatedAt:   value.StartedAt,
		UpdatedAt:   value.UpdatedAt,
		Favorite:    value.Favorite,
		Revision:    value.Revision,
	}, nil
}
