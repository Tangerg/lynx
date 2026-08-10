package mock

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (r *Runtime) ListSessions(ctx context.Context, query agent.SessionQuery) (agent.SessionPage, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.SessionPage{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]agent.Session, 0, len(r.sessions))
	needle := strings.ToLower(strings.TrimSpace(query.Search))
	workspace := strings.TrimSpace(query.Workspace)
	for _, state := range r.sessions {
		if workspace != "" && state.meta.Workspace != workspace {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(state.meta.Title+"\n"+state.meta.Workspace), needle) {
			continue
		}
		items = append(items, state.meta)
	}
	slices.SortStableFunc(items, func(a, b agent.Session) int { return b.UpdatedAt.Compare(a.UpdatedAt) })

	offset, err := pageOffset(query.Cursor, len(items))
	if err != nil {
		return agent.SessionPage{}, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = defaultPageSize
	}
	limit = min(limit, maxPageSize)
	end := min(offset+limit, len(items))
	page := agent.SessionPage{Items: slices.Clone(items[offset:end])}
	if end < len(items) {
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}

func pageOffset(cursor string, length int) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 || offset > length {
		return 0, fmt.Errorf("mock: invalid session page cursor %q", cursor)
	}
	return offset, nil
}

func (r *Runtime) GetSession(ctx context.Context, id string) (agent.SessionSnapshot, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.SessionSnapshot{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.sessions[id]
	if !ok {
		return agent.SessionSnapshot{}, fmt.Errorf("%w: %s", agent.ErrSessionNotFound, id)
	}
	snapshot := agent.SessionSnapshot{
		Session: state.meta,
		Events:  cloneEnvelopes(state.events),
		Cursor:  agent.Cursor(len(state.events)),
	}
	if active, ok := r.runs[state.active]; ok {
		run := projectRun(active)
		snapshot.Active = &run
	}
	if err := snapshot.Validate(); err != nil {
		return agent.SessionSnapshot{}, fmt.Errorf("mock: invalid session snapshot: %w", err)
	}
	return snapshot, nil
}

func (r *Runtime) CreateSession(ctx context.Context, in agent.CreateSession) (agent.Session, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.Session{}, err
	}
	workspace := strings.TrimSpace(in.Workspace)
	if workspace == "" {
		return agent.Session{}, errors.New("mock: workspace is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "Untitled session"
	}
	session := agent.Session{
		ID: fmt.Sprintf("ses_mock_%d", r.next), Title: title, Workspace: workspace,
		UpdatedAt: r.now(), Revision: 1,
	}
	r.sessions[session.ID] = &sessionState{meta: session, changed: make(chan struct{})}
	return session, nil
}

func (r *Runtime) UpdateSession(ctx context.Context, in agent.UpdateSession) (agent.Session, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.Session{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.sessions[in.SessionID]
	if !ok {
		return agent.Session{}, fmt.Errorf("%w: %s", agent.ErrSessionNotFound, in.SessionID)
	}
	if in.Revision != 0 && in.Revision != state.meta.Revision {
		return agent.Session{}, fmt.Errorf("%w: session %s is at revision %d", agent.ErrRevisionConflict, in.SessionID, state.meta.Revision)
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return agent.Session{}, errors.New("mock: session title is empty")
	}
	state.meta.Title = title
	state.meta.Revision++
	state.meta.UpdatedAt = r.now()
	return state.meta, nil
}

func (r *Runtime) ForkSession(ctx context.Context, in agent.ForkSession) (agent.Session, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.Session{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	source, ok := r.sessions[in.SessionID]
	if !ok {
		return agent.Session{}, fmt.Errorf("%w: %s", agent.ErrSessionNotFound, in.SessionID)
	}
	if source.starting != nil {
		return agent.Session{}, fmt.Errorf("%w: %s", agent.ErrSessionBusy, in.SessionID)
	}
	at := in.At
	if at == 0 {
		at = agent.Cursor(len(source.events))
	}
	if at > agent.Cursor(len(source.events)) {
		return agent.Session{}, fmt.Errorf("mock: fork cursor %d is beyond session cursor %d", at, len(source.events))
	}
	prefix := agent.SessionSnapshot{Session: source.meta, Events: cloneEnvelopes(source.events[:at]), Cursor: at}
	if err := prefix.Validate(); err != nil {
		return agent.Session{}, fmt.Errorf("mock: fork cursor %d is not a settled run boundary: %w", at, err)
	}
	r.next++
	id := fmt.Sprintf("ses_mock_%d", r.next)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = source.meta.Title + " (fork)"
	}
	meta := agent.Session{
		ID: id, Title: title, Workspace: source.meta.Workspace,
		UpdatedAt: r.now(), Revision: 1,
	}
	state := &sessionState{meta: meta, changed: make(chan struct{})}
	for _, sourceEnvelope := range source.events[:at] {
		r.next++
		envelope := cloneEnvelope(sourceEnvelope)
		envelope.ID = fmt.Sprintf("evt_mock_%d", r.next)
		envelope.SessionID = id
		if started, ok := envelope.Event.(agent.RunStarted); ok {
			started.SessionID = id
			envelope.Event = started
		}
		state.events = append(state.events, envelope)
	}
	r.sessions[id] = state
	return meta, nil
}

func (r *Runtime) DeleteSession(ctx context.Context, in agent.DeleteSession) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.sessions[in.SessionID]
	if !ok {
		return fmt.Errorf("%w: %s", agent.ErrSessionNotFound, in.SessionID)
	}
	if state.active != "" || state.starting != nil {
		return fmt.Errorf("%w: %s", agent.ErrSessionBusy, in.SessionID)
	}
	if in.Revision != 0 && in.Revision != state.meta.Revision {
		return fmt.Errorf("%w: session %s is at revision %d", agent.ErrRevisionConflict, in.SessionID, state.meta.Revision)
	}
	delete(r.sessions, in.SessionID)
	return nil
}

func (r *Runtime) seedHistory() {
	state := r.sessions["ses_demo_1"]
	if state == nil {
		return
	}
	run := &runState{id: "run_demo_history", sessionID: state.meta.ID, status: agent.RunComplete}
	r.runs[run.id] = run
	for _, event := range []agent.Event{
		agent.RunStarted{RunID: run.id, SessionID: state.meta.ID, Options: agent.RunOptions{Model: "mock-balanced", Mode: agent.ModeBuild, Permission: agent.PermissionAsk, Effort: "medium"}},
		agent.BlockCompleted{Block: agent.Block{ID: "demo_prompt", Kind: agent.BlockUser, Text: "Why is the cache expiry test flaky?"}},
		agent.BlockCompleted{Block: agent.Block{ID: "demo_answer", Kind: agent.BlockAssistant, Text: "The fixed sleep races the janitor. Wait for its sweep signal instead."}},
		agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}, Usage: agent.Usage{InputTokens: 820, OutputTokens: 94, CachedTokens: 512, Duration: 3 * time.Second}},
	} {
		r.emitLocked(run, event)
	}
}
