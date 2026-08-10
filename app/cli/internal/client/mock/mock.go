package mock

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

var errCanceled = errors.New("mock: run canceled")

const (
	defaultPageSize = 20
	maximumPageSize = 100
)

// FaultKind identifies one transport fault injected into the next run
// subscription. Faults alter delivery only; the durable session log remains
// intact so clients can prove their replay and recovery behavior.
type FaultKind string

const (
	FaultDisconnect FaultKind = "disconnect"
	FaultDuplicate  FaultKind = "duplicate"
	FaultGap        FaultKind = "gap"
	FaultConflict   FaultKind = "conflict"
)

// SubscriptionFault is consumed by one FollowRun call. After is the one-based
// delivery position at which the fault occurs; values below one mean the first
// event.
type SubscriptionFault struct {
	Kind  FaultKind
	After int
}

// Runtime is a complete in-memory runtime adapter. Runs outlive individual
// subscriptions, event logs are replayable, and every operation is safe for
// concurrent use so the mock exercises the same lifecycle as a remote adapter.
type Runtime struct {
	Instant bool
	Script  func(prompt string) Script
	Faults  []SubscriptionFault

	mu       sync.Mutex
	sessions map[string]*sessionState
	runs     map[string]*runState
	starts   map[string]*startAttempt
	canceled map[string]struct{}
	rules    []client.ApprovalRule
	fault    int
	next     uint64
	now      func() time.Time
}

type sessionState struct {
	meta     client.Session
	events   []client.Envelope
	active   string
	starting *startAttempt
	changed  chan struct{}
}

type startAttempt struct {
	input    client.StartRun
	ready    chan struct{}
	run      client.Run
	err      error
	finished bool
}

type runState struct {
	id           string
	sessionID    string
	startedAfter client.Cursor
	status       client.RunStatus
	script       Script
	interaction  client.Interaction
	start        client.StartRun
	answers      map[string]client.Answer
	resuming     *resumeAttempt
	cancel       chan struct{}
	cancelOnce   sync.Once
	finishOnce   sync.Once
}

type resumeAttempt struct {
	interruptID string
	answer      client.Answer
	ready       chan struct{}
	err         error
}

func New() *Runtime {
	r := &Runtime{
		sessions: make(map[string]*sessionState),
		runs:     make(map[string]*runState),
		starts:   make(map[string]*startAttempt),
		canceled: make(map[string]struct{}),
		now:      time.Now,
	}
	for _, session := range demoSessions() {
		r.sessions[session.ID] = &sessionState{meta: session, changed: make(chan struct{})}
	}
	r.seedHistory()
	return r
}

var _ client.Runtime = (*Runtime)(nil)

func (r *Runtime) ListSessions(_ context.Context, query client.SessionQuery) (client.SessionPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]client.Session, 0, len(r.sessions))
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
	slices.SortStableFunc(items, func(a, b client.Session) int { return b.UpdatedAt.Compare(a.UpdatedAt) })

	offset, err := pageOffset(query.Cursor, len(items))
	if err != nil {
		return client.SessionPage{}, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = defaultPageSize
	}
	limit = min(limit, maximumPageSize)
	end := min(offset+limit, len(items))
	page := client.SessionPage{Items: slices.Clone(items[offset:end])}
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

func (r *Runtime) GetSession(_ context.Context, id string) (client.SessionSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.sessions[id]
	if !ok {
		return client.SessionSnapshot{}, fmt.Errorf("%w: %s", client.ErrSessionNotFound, id)
	}
	snapshot := client.SessionSnapshot{
		Session: state.meta,
		Events:  cloneEnvelopes(state.events),
		Cursor:  client.Cursor(len(state.events)),
	}
	if active, ok := r.runs[state.active]; ok {
		run := projectRun(active)
		snapshot.Active = &run
	}
	if err := snapshot.Validate(); err != nil {
		return client.SessionSnapshot{}, fmt.Errorf("mock: invalid session snapshot: %w", err)
	}
	return snapshot, nil
}

func (r *Runtime) CreateSession(_ context.Context, in client.NewSession) (client.Session, error) {
	workspace := strings.TrimSpace(in.Workspace)
	if workspace == "" {
		return client.Session{}, errors.New("mock: workspace is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "Untitled session"
	}
	session := client.Session{
		ID: fmt.Sprintf("ses_mock_%d", r.next), Title: title, Workspace: workspace,
		UpdatedAt: r.now(), Revision: 1,
	}
	r.sessions[session.ID] = &sessionState{meta: session, changed: make(chan struct{})}
	return session, nil
}

func (r *Runtime) UpdateSession(_ context.Context, in client.UpdateSession) (client.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.sessions[in.SessionID]
	if !ok {
		return client.Session{}, fmt.Errorf("%w: %s", client.ErrSessionNotFound, in.SessionID)
	}
	if in.Revision != 0 && in.Revision != state.meta.Revision {
		return client.Session{}, fmt.Errorf("%w: session %s is at revision %d", client.ErrRevisionConflict, in.SessionID, state.meta.Revision)
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return client.Session{}, errors.New("mock: session title is empty")
	}
	state.meta.Title = title
	state.meta.Revision++
	state.meta.UpdatedAt = r.now()
	return state.meta, nil
}

func (r *Runtime) ForkSession(_ context.Context, in client.ForkSession) (client.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	source, ok := r.sessions[in.SessionID]
	if !ok {
		return client.Session{}, fmt.Errorf("%w: %s", client.ErrSessionNotFound, in.SessionID)
	}
	if source.starting != nil {
		return client.Session{}, fmt.Errorf("%w: %s", client.ErrSessionBusy, in.SessionID)
	}
	at := in.At
	if at == 0 {
		at = client.Cursor(len(source.events))
	}
	if at > client.Cursor(len(source.events)) {
		return client.Session{}, fmt.Errorf("mock: fork cursor %d is beyond session cursor %d", at, len(source.events))
	}
	prefix := client.SessionSnapshot{Session: source.meta, Events: cloneEnvelopes(source.events[:at]), Cursor: at}
	if err := prefix.Validate(); err != nil {
		return client.Session{}, fmt.Errorf("mock: fork cursor %d is not a settled run boundary: %w", at, err)
	}
	r.next++
	id := fmt.Sprintf("ses_mock_%d", r.next)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = source.meta.Title + " (fork)"
	}
	meta := client.Session{
		ID: id, Title: title, Workspace: source.meta.Workspace,
		UpdatedAt: r.now(), Revision: 1,
	}
	state := &sessionState{meta: meta, changed: make(chan struct{})}
	for _, sourceEnvelope := range source.events[:at] {
		r.next++
		envelope := cloneEnvelope(sourceEnvelope)
		envelope.ID = fmt.Sprintf("evt_mock_%d", r.next)
		envelope.SessionID = id
		if started, ok := envelope.Event.(client.RunStarted); ok {
			started.SessionID = id
			envelope.Event = started
		}
		state.events = append(state.events, envelope)
	}
	r.sessions[id] = state
	return meta, nil
}

func (r *Runtime) DeleteSession(_ context.Context, in client.DeleteSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.sessions[in.SessionID]
	if !ok {
		return fmt.Errorf("%w: %s", client.ErrSessionNotFound, in.SessionID)
	}
	if state.active != "" || state.starting != nil {
		return fmt.Errorf("%w: %s", client.ErrSessionBusy, in.SessionID)
	}
	if in.Revision != 0 && in.Revision != state.meta.Revision {
		return fmt.Errorf("%w: session %s is at revision %d", client.ErrRevisionConflict, in.SessionID, state.meta.Revision)
	}
	delete(r.sessions, in.SessionID)
	return nil
}

func (r *Runtime) ListModels(context.Context) ([]client.Model, error) {
	return []client.Model{
		{ID: "mock-balanced", DisplayName: "Mock Balanced", Description: "Synthetic balanced coding model", Default: true, Efforts: []string{"low", "medium", "high"}, Context: 200_000},
		{ID: "mock-fast", DisplayName: "Mock Fast", Description: "Synthetic low-latency model", Efforts: []string{"low", "medium"}, Context: 128_000},
		{ID: "mock-deep", DisplayName: "Mock Deep", Description: "Synthetic deep-reasoning model", Efforts: []string{"medium", "high", "max"}, Context: 400_000},
	}, nil
}

func (r *Runtime) ListApprovalRules(context.Context) ([]client.ApprovalRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.rules)
	slices.Reverse(out)
	return out, nil
}

func (r *Runtime) DeleteApprovalRule(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	at := slices.IndexFunc(r.rules, func(rule client.ApprovalRule) bool { return rule.ID == id })
	if at < 0 {
		return fmt.Errorf("mock: approval rule %s not found", id)
	}
	r.rules = slices.Delete(r.rules, at, at+1)
	return nil
}

func (r *Runtime) StartRun(ctx context.Context, in client.StartRun) (client.Run, error) {
	if err := in.Validate(); err != nil {
		return client.Run{}, fmt.Errorf("mock: %w", err)
	}
	attempt, owner, err := r.reserveStart(ctx, in)
	if err != nil {
		return client.Run{}, err
	}
	if !owner {
		return awaitStart(ctx, r, attempt)
	}

	build := r.Script
	if build == nil {
		build = Conversation
	}
	script, buildErr := buildScriptSafely(build, strings.TrimSpace(in.Message.Text))

	r.mu.Lock()
	run, err := r.commitStartLocked(attempt, script, buildErr)
	started := client.Run{}
	if err == nil {
		started = projectRun(run)
	}
	r.mu.Unlock()
	if err != nil {
		return client.Run{}, err
	}
	go r.play(run, run.script.Prelude, run.script.interrupts())
	return started, nil
}

func (r *Runtime) reserveStart(ctx context.Context, in client.StartRun) (*startAttempt, bool, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[in.SessionID]
	if !ok {
		return nil, false, fmt.Errorf("%w: %s", client.ErrSessionNotFound, in.SessionID)
	}
	if in.RequestID != "" {
		key := requestKey(in.SessionID, in.RequestID)
		if _, canceled := r.canceled[key]; canceled {
			return nil, false, fmt.Errorf("%w: request %s", client.ErrRunCanceled, in.RequestID)
		}
		if existing := r.starts[key]; existing != nil {
			if !sameStart(existing.input, in) {
				return nil, false, fmt.Errorf("%w: request %s", client.ErrRequestConflict, in.RequestID)
			}
			return existing, false, nil
		}
	}
	if session.active != "" || session.starting != nil {
		return nil, false, fmt.Errorf("%w: %s", client.ErrSessionBusy, in.SessionID)
	}
	attempt := &startAttempt{input: cloneStart(in), ready: make(chan struct{})}
	session.starting = attempt
	if in.RequestID != "" {
		r.starts[requestKey(in.SessionID, in.RequestID)] = attempt
	}
	return attempt, true, nil
}

func awaitStart(ctx context.Context, runtime *Runtime, attempt *startAttempt) (client.Run, error) {
	select {
	case <-attempt.ready:
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return attempt.run, attempt.err
	case <-ctx.Done():
		return client.Run{}, context.Cause(ctx)
	}
}

func (r *Runtime) commitStartLocked(attempt *startAttempt, script Script, buildErr error) (*runState, error) {
	if attempt.finished {
		return nil, attempt.err
	}
	session := r.sessions[attempt.input.SessionID]
	if session == nil || session.starting != attempt {
		err := fmt.Errorf("%w: request %s", client.ErrRunCanceled, attempt.input.RequestID)
		r.finishStartLocked(attempt, client.Run{}, err)
		return nil, err
	}
	if buildErr != nil {
		err := fmt.Errorf("mock: build script: %w", buildErr)
		session.starting = nil
		r.finishStartLocked(attempt, client.Run{}, err)
		return nil, err
	}
	r.next++
	run := &runState{
		id: fmt.Sprintf("run_mock_%d", r.next), sessionID: attempt.input.SessionID,
		startedAfter: client.Cursor(len(session.events)), status: client.RunActive,
		script: script, start: cloneStart(attempt.input), answers: make(map[string]client.Answer),
		cancel: make(chan struct{}),
	}
	r.runs[run.id] = run
	session.starting = nil
	session.active = run.id
	r.emitLocked(run, client.RunStarted{RunID: run.id, SessionID: run.sessionID, Options: attempt.input.Options})
	r.emitLocked(run, client.BlockCompleted{Block: client.Block{
		ID: run.id + "_prompt", Kind: client.BlockUser,
		Text: strings.TrimSpace(attempt.input.Message.Text), Attachments: slices.Clone(attempt.input.Message.Attachments),
	}})
	started := projectRun(run)
	r.finishStartLocked(attempt, started, nil)
	return run, nil
}

func (r *Runtime) finishStartLocked(attempt *startAttempt, run client.Run, err error) {
	if attempt.finished {
		return
	}
	attempt.run, attempt.err, attempt.finished = run, err, true
	close(attempt.ready)
}

func (r *Runtime) FollowRun(ctx context.Context, in client.FollowRun) (client.Stream, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("mock: %w", err)
	}
	r.mu.Lock()
	run, ok := r.runs[in.RunID]
	if !ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", client.ErrRunNotFound, in.RunID)
	}
	if in.After < run.startedAfter {
		r.mu.Unlock()
		return nil, fmt.Errorf("mock: cursor %d predates run start cursor %d", in.After, run.startedAfter)
	}
	session := r.sessions[run.sessionID]
	latest := client.Cursor(len(session.events))
	if in.After > latest {
		r.mu.Unlock()
		return nil, fmt.Errorf("mock: cursor %d is after session cursor %d", in.After, latest)
	}
	fault, err := r.takeFaultLocked()
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}

	return func(yield func(client.Envelope, error) bool) {
		after := in.After
		position := 0
		for {
			r.mu.Lock()
			session := r.sessions[run.sessionID]
			var next *client.Envelope
			for _, envelope := range session.events {
				if envelope.Cursor <= after || envelope.RunID != run.id {
					continue
				}
				cloned := cloneEnvelope(envelope)
				next = &cloned
				break
			}
			status := run.status
			changed := session.changed
			r.mu.Unlock()

			if next != nil {
				position++
				if fault.Kind == FaultGap && position == fault.After {
					after = next.Cursor
					continue
				}
				after = next.Cursor
				if !yield(*next, nil) {
					return
				}
				if position == fault.After {
					switch fault.Kind {
					case FaultDuplicate:
						if !yield(*next, nil) {
							return
						}
					case FaultConflict:
						conflict := cloneEnvelope(*next)
						conflict.ID += "_conflict"
						yield(conflict, nil)
						return
					}
				}
				switch next.Event.(type) {
				case client.RunInterrupted, client.RunFinished:
					return
				}
				if fault.Kind == FaultDisconnect && position == fault.After {
					yield(client.Envelope{}, fmt.Errorf("%w after cursor %d", client.ErrDisconnected, after))
					return
				}
				continue
			}
			if status != client.RunActive {
				return
			}
			select {
			case <-changed:
			case <-ctx.Done():
				if !yield(client.Envelope{}, context.Canceled) {
					return
				}
				return
			}
		}
	}, nil
}

func (r *Runtime) takeFaultLocked() (SubscriptionFault, error) {
	if r.fault >= len(r.Faults) {
		return SubscriptionFault{}, nil
	}
	fault := r.Faults[r.fault]
	r.fault++
	if fault.After < 1 {
		fault.After = 1
	}
	switch fault.Kind {
	case FaultDisconnect, FaultDuplicate, FaultGap, FaultConflict:
		return fault, nil
	default:
		return SubscriptionFault{}, fmt.Errorf("mock: unknown subscription fault %q", fault.Kind)
	}
}

func (r *Runtime) ResumeRun(ctx context.Context, in client.ResumeRun) error {
	if err := in.Validate(); err != nil {
		return fmt.Errorf("mock: %w", err)
	}
	run, attempt, owner, err := r.reserveResume(ctx, in)
	if err != nil || attempt == nil {
		return err
	}
	if !owner {
		return awaitResume(ctx, r, attempt)
	}
	steps, continuationErr := continueSafely(run.script, in.Answer)
	r.mu.Lock()
	err = r.commitResumeLocked(run, attempt, steps, continuationErr)
	r.mu.Unlock()
	if err != nil {
		return err
	}
	go r.play(run, steps, false)
	return nil
}

func (r *Runtime) reserveResume(ctx context.Context, in client.ResumeRun) (*runState, *resumeAttempt, bool, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, nil, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[in.RunID]
	if !ok {
		return nil, nil, false, fmt.Errorf("%w: %s", client.ErrRunNotFound, in.RunID)
	}
	attempt, owner, err := resolveResumeAttemptLocked(run, in)
	return run, attempt, owner, err
}

func resolveResumeAttemptLocked(run *runState, in client.ResumeRun) (*resumeAttempt, bool, error) {
	if answered, exists := run.answers[in.InterruptID]; exists {
		if client.EqualAnswers(answered, in.Answer) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("%w: interrupt %s", client.ErrRequestConflict, in.InterruptID)
	}
	if pending := run.resuming; pending != nil {
		if pending.interruptID != in.InterruptID || !client.EqualAnswers(pending.answer, in.Answer) {
			return nil, false, fmt.Errorf("%w: interrupt %s", client.ErrRequestConflict, in.InterruptID)
		}
		return pending, false, nil
	}
	if run.status != client.RunWaiting || client.InteractionID(run.interaction) != in.InterruptID {
		return nil, false, fmt.Errorf("%w: %s", client.ErrInterruptNotOpen, in.InterruptID)
	}
	if err := client.ValidateAnswer(run.interaction, in.Answer); err != nil {
		return nil, false, fmt.Errorf("mock: %w", err)
	}
	attempt := &resumeAttempt{
		interruptID: in.InterruptID,
		answer:      client.CloneAnswer(in.Answer),
		ready:       make(chan struct{}),
	}
	run.resuming = attempt
	return attempt, true, nil
}

func awaitResume(ctx context.Context, runtime *Runtime, attempt *resumeAttempt) error {
	select {
	case <-attempt.ready:
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return attempt.err
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (r *Runtime) commitResumeLocked(run *runState, attempt *resumeAttempt, steps []Step, continuationErr error) error {
	if run.resuming != attempt {
		return attempt.err
	}
	if continuationErr != nil {
		err := fmt.Errorf("mock: continue script: %w", continuationErr)
		r.finishResumeLocked(run, attempt, err)
		return err
	}
	if run.status != client.RunWaiting {
		err := fmt.Errorf("%w: run %s", client.ErrRunCanceled, run.id)
		r.finishResumeLocked(run, attempt, err)
		return err
	}
	run.answers[attempt.interruptID] = client.CloneAnswer(attempt.answer)
	if approval, ok := run.interaction.(client.Approval); ok {
		if answer, ok := attempt.answer.(client.ApprovalAnswer); ok && answer.Remember != "" && answer.Remember != client.RememberNone {
			r.rememberApprovalLocked(run, approval, answer)
		}
	}
	run.status = client.RunActive
	run.interaction = nil
	r.emitLocked(run, client.RunResumed{InterruptID: attempt.interruptID})
	r.finishResumeLocked(run, attempt, nil)
	return nil
}

func (r *Runtime) finishResumeLocked(run *runState, attempt *resumeAttempt, err error) {
	if run.resuming == attempt {
		run.resuming = nil
	}
	attempt.err = err
	close(attempt.ready)
}

func (r *Runtime) CancelRun(ctx context.Context, in client.CancelRun) error {
	if err := in.Validate(); err != nil {
		return fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run, err := r.resolveCancellationLocked(in)
	if err != nil || run == nil {
		return err
	}
	r.cancelRunLocked(run)
	return nil
}

func (r *Runtime) resolveCancellationLocked(in client.CancelRun) (*runState, error) {
	if in.RunID != "" {
		run := r.runs[in.RunID]
		if run == nil {
			return nil, fmt.Errorf("%w: %s", client.ErrRunNotFound, in.RunID)
		}
		return run, nil
	}
	return r.cancelStartLocked(in)
}

func (r *Runtime) cancelStartLocked(in client.CancelRun) (*runState, error) {
	session := r.sessions[in.SessionID]
	if session == nil {
		return nil, fmt.Errorf("%w: %s", client.ErrSessionNotFound, in.SessionID)
	}
	key := requestKey(in.SessionID, in.RequestID)
	r.canceled[key] = struct{}{}
	attempt := r.starts[key]
	if attempt == nil {
		return nil, nil
	}
	if !attempt.finished {
		if session.starting == attempt {
			session.starting = nil
		}
		r.finishStartLocked(attempt, client.Run{}, fmt.Errorf("%w: request %s", client.ErrRunCanceled, in.RequestID))
		return nil, nil
	}
	if attempt.err != nil {
		return nil, nil
	}
	return r.runs[attempt.run.ID], nil
}

func (r *Runtime) cancelRunLocked(run *runState) {
	run.cancelOnce.Do(func() { close(run.cancel) })
	if pending := run.resuming; pending != nil {
		r.finishResumeLocked(run, pending, fmt.Errorf("%w: run %s", client.ErrRunCanceled, run.id))
	}
	r.finishLocked(run, client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCanceled}})
}

func (r *Runtime) play(run *runState, steps []Step, interrupt bool) {
	if !r.playSteps(run, steps) || !interrupt {
		return
	}
	r.park(run)
}

func (r *Runtime) playSteps(run *runState, steps []Step) bool {
	for _, step := range steps {
		if err := r.pause(run, step.Delay); err != nil {
			if errors.Is(err, errCanceled) {
				r.finish(run, client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCanceled}})
			}
			return false
		}
		if finished, done := step.Event.(client.RunFinished); done {
			r.finish(run, finished)
			return false
		}
		if !r.emit(run, step.Event) {
			return false
		}
	}
	return true
}

func (r *Runtime) park(run *runState) {
	r.mu.Lock()
	if run.status != client.RunActive {
		r.mu.Unlock()
		return
	}
	var remembered *client.ApprovalAnswer
	if approval, ok := run.script.Interaction.(client.Approval); ok {
		if answer, matched := r.rememberedAnswerLocked(run, approval); matched {
			remembered = &answer
		}
	}
	if remembered != nil {
		approval := run.script.Interaction.(client.Approval)
		r.mu.Unlock()
		steps, err := continueSafely(run.script, *remembered)
		if err != nil {
			r.finish(run, client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeFailed, Error: "mock continuation: " + err.Error()}})
			return
		}
		r.mu.Lock()
		if run.status != client.RunActive {
			r.mu.Unlock()
			return
		}
		r.emitLocked(run, client.BlockCompleted{Block: client.Block{
			ID: run.id + "_approval_rule", Kind: client.BlockNotice,
			Text: "Applied remembered approval rule: " + approvalRuleKey(approval),
		}})
		r.mu.Unlock()
		r.play(run, steps, false)
		return
	}
	run.status = client.RunWaiting
	run.interaction = client.CloneInteraction(run.script.Interaction)
	r.emitLocked(run, client.RunInterrupted{Interaction: client.CloneInteraction(run.interaction)})
	r.mu.Unlock()
}

func (r *Runtime) rememberApprovalLocked(run *runState, approval client.Approval, answer client.ApprovalAnswer) {
	session := r.sessions[run.sessionID]
	key := approvalRuleKey(approval)
	for _, rule := range r.rules {
		if rule.Rule == key && rule.Scope == answer.Remember && ruleApplies(rule, run.sessionID, session.meta.Workspace) {
			return
		}
	}
	r.next++
	rule := client.ApprovalRule{
		ID: fmt.Sprintf("rule_mock_%d", r.next), Rule: key, Decision: answer.Decision,
		Scope: answer.Remember, CreatedAt: r.now(),
	}
	switch answer.Remember {
	case client.RememberSession:
		rule.SessionID = run.sessionID
	case client.RememberProject:
		rule.Workspace = session.meta.Workspace
	}
	r.rules = append(r.rules, rule)
}

func (r *Runtime) rememberedAnswerLocked(run *runState, approval client.Approval) (client.ApprovalAnswer, bool) {
	workspace := r.sessions[run.sessionID].meta.Workspace
	key := approvalRuleKey(approval)
	for _, rule := range slices.Backward(r.rules) {
		if rule.Rule == key && ruleApplies(rule, run.sessionID, workspace) {
			return client.ApprovalAnswer{Decision: rule.Decision, Remember: rule.Scope}, true
		}
	}
	return client.ApprovalAnswer{}, false
}

func ruleApplies(rule client.ApprovalRule, sessionID, workspace string) bool {
	switch rule.Scope {
	case client.RememberSession:
		return rule.SessionID == sessionID
	case client.RememberProject:
		return rule.Workspace == workspace
	case client.RememberGlobal:
		return true
	default:
		return false
	}
}

func approvalRuleKey(approval client.Approval) string {
	if key := strings.TrimSpace(approval.RuleHint); key != "" {
		return key
	}
	return strings.TrimSpace(approval.Title)
}

func (r *Runtime) pause(run *runState, delay time.Duration) error {
	if r.Instant || delay <= 0 {
		select {
		case <-run.cancel:
			return errCanceled
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-run.cancel:
		return errCanceled
	}
}

func (r *Runtime) emit(run *runState, event client.Event) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.status != client.RunActive {
		return false
	}
	r.emitLocked(run, event)
	return true
}

func (r *Runtime) emitLocked(run *runState, event client.Event) {
	session := r.sessions[run.sessionID]
	r.next++
	envelope := client.Envelope{
		ID: fmt.Sprintf("evt_mock_%d", r.next), Cursor: client.Cursor(len(session.events) + 1),
		RunID: run.id, SessionID: run.sessionID, At: r.now(), Event: cloneEvent(event),
	}
	session.events = append(session.events, envelope)
	session.meta.UpdatedAt = envelope.At
	session.meta.Revision++
	if _, ok := event.(client.RunFinished); ok {
		run.status = client.RunComplete
		session.active = ""
	}
	close(session.changed)
	session.changed = make(chan struct{})
}

func (r *Runtime) finish(run *runState, event client.RunFinished) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishLocked(run, event)
}

func (r *Runtime) finishLocked(run *runState, event client.RunFinished) {
	run.finishOnce.Do(func() { r.emitLocked(run, event) })
}

func projectRun(run *runState) client.Run {
	return client.Run{ID: run.id, SessionID: run.sessionID, Status: run.status, StartedAfter: run.startedAfter}
}

func requestKey(sessionID, requestID string) string {
	return sessionID + "\x00" + requestID
}

func cloneStart(start client.StartRun) client.StartRun {
	start.Message.Text = strings.Clone(start.Message.Text)
	start.Message.Attachments = slices.Clone(start.Message.Attachments)
	return start
}

func sameStart(a, b client.StartRun) bool {
	return a.RequestID == b.RequestID &&
		a.SessionID == b.SessionID &&
		a.Message.Text == b.Message.Text &&
		a.Options == b.Options &&
		slices.Equal(a.Message.Attachments, b.Message.Attachments)
}

func cloneEnvelopes(events []client.Envelope) []client.Envelope {
	out := make([]client.Envelope, len(events))
	for i, envelope := range events {
		out[i] = cloneEnvelope(envelope)
	}
	return out
}

func cloneEnvelope(envelope client.Envelope) client.Envelope {
	envelope.Event = cloneEvent(envelope.Event)
	return envelope
}

func cloneEvent(event client.Event) client.Event {
	switch item := event.(type) {
	case client.RunStarted:
		return item
	case client.RunResumed:
		return item
	case client.BlockStarted:
		item.Block = cloneBlock(item.Block)
		return item
	case client.BlockDelta:
		return item
	case client.BlockCompleted:
		item.Block = cloneBlock(item.Block)
		return item
	case client.PlanChanged:
		item.Items = slices.Clone(item.Items)
		return item
	case client.RunInterrupted:
		item.Interaction = client.CloneInteraction(item.Interaction)
		return item
	case client.RunFinished:
		return item
	default:
		return nil
	}
}

func cloneBlock(block client.Block) client.Block {
	block.Attachments = slices.Clone(block.Attachments)
	if block.Tool != nil {
		tool := *block.Tool
		if block.Tool.ExitCode != nil {
			code := *block.Tool.ExitCode
			tool.ExitCode = &code
		}
		block.Tool = &tool
	}
	return block
}

func (r *Runtime) seedHistory() {
	state := r.sessions["ses_demo_1"]
	if state == nil {
		return
	}
	run := &runState{id: "run_demo_history", sessionID: state.meta.ID, status: client.RunComplete}
	r.runs[run.id] = run
	for _, event := range []client.Event{
		client.RunStarted{RunID: run.id, SessionID: state.meta.ID, Options: client.RunOptions{Model: "mock-balanced", Mode: client.ModeBuild, Permission: client.PermissionAsk, Effort: "medium"}},
		client.BlockCompleted{Block: client.Block{ID: "demo_prompt", Kind: client.BlockUser, Text: "Why is the cache expiry test flaky?"}},
		client.BlockCompleted{Block: client.Block{ID: "demo_answer", Kind: client.BlockAssistant, Text: "The fixed sleep races the janitor. Wait for its sweep signal instead."}},
		client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}, Usage: client.Usage{InputTokens: 820, OutputTokens: 94, CachedTokens: 512, Duration: 3 * time.Second}},
	} {
		r.emitLocked(run, event)
	}
}
