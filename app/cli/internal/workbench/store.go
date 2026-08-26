// Package workbench owns durable, CLI-local authoring state. It deliberately
// knows nothing about terminal widgets or runtime persistence.
package workbench

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/spf13/pathologize"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const (
	formatVersion        = 1
	defaultHistoryLimit  = 1000
	defaultStashLimit    = 100
	defaultWorkspaceCap  = 50
	stashTransferName    = "stash-transfer.json"
	sessionDeletionsName = "session-deletions.json"
)

// Config controls bounded state and supplies deterministic identity sources to
// tests. Zero values select production defaults.
type Config struct {
	HistoryLimit   int
	StashLimit     int
	WorkspaceLimit int
	Now            func() time.Time
	Random         io.Reader
}

// Stash is an explicitly named prompt snapshot.
type Stash struct {
	ID        string        `json:"id"`
	CreatedAt time.Time     `json:"createdAt"`
	Message   agent.Message `json:"message"`
}

// stashTransfer is the durable intent for the only workbench mutation that
// spans the global stash catalog and one session aggregate. Recovery uses the
// draft value as an ownership precondition: it completes an interrupted move
// only while that exact draft still occupies the source session.
type stashTransfer struct {
	SessionID string        `json:"sessionId"`
	Draft     agent.Message `json:"draft"`
	Stash     Stash         `json:"stash"`
}

func (s stashTransfer) validate() error {
	if strings.TrimSpace(s.SessionID) == "" {
		return errors.New("stash transfer session id is empty")
	}
	if messageEmpty(s.Draft) || messageEmpty(s.Stash.Message) {
		return errors.New("stash transfer prompt is empty")
	}
	identity, err := hex.DecodeString(s.Stash.ID)
	if err != nil || len(identity) != 8 || s.Stash.CreatedAt.IsZero() ||
		!s.Stash.Message.Equal(s.Draft) {
		return errors.New("stash transfer identity or prompt is inconsistent")
	}
	return nil
}

func stashEqual(left, right Stash) bool {
	return left.ID == right.ID && left.CreatedAt.Equal(right.CreatedAt) && left.Message.Equal(right.Message)
}

// Workspace is one recently used authoring root.
type Workspace struct {
	Path       string    `json:"path"`
	LastOpened time.Time `json:"lastOpened"`
}

// historyEntry binds a runtime-accepted prompt to its mutation identity. Plain
// authoring history intentionally has no identity; accepted starts use it to
// make the history half of outbox settlement idempotent across process or
// filesystem failure between the two durable files.
type historyEntry struct {
	agent.Message
	CommandID agent.CommandID `json:"commandId,omitempty"`
}

type sessionState struct {
	SessionID       string                  `json:"sessionId"`
	Draft           agent.Message           `json:"draft"`
	PendingRuns     []PendingRun            `json:"pendingRuns"`
	PendingResume   *PendingResume          `json:"pendingResume,omitempty"`
	PendingRollback *PendingSessionRollback `json:"pendingRollback,omitempty"`
	PendingSteer    *PendingSteer           `json:"pendingSteer,omitempty"`
}

// SessionDeletionPhase separates an unacknowledged runtime mutation from a
// confirmed local tombstone. A confirmed record makes an obsolete session
// aggregate unreachable even when its physical file cannot yet be removed.
type SessionDeletionPhase string

const (
	SessionDeletionPrepared  SessionDeletionPhase = "prepared"
	SessionDeletionConfirmed SessionDeletionPhase = "confirmed"
)

// PendingSessionDeletion is the durable journal for one session deletion.
// Prepared records retain the stable runtime command identity; confirmed
// records remain only while obsolete CLI-local state still needs cleanup.
type PendingSessionDeletion struct {
	Phase     SessionDeletionPhase `json:"phase"`
	CommandID agent.CommandID      `json:"commandId,omitempty"`
	SessionID string               `json:"sessionId"`
	Replay    ReplayGuard          `json:"replay"`
}

func (p PendingSessionDeletion) validate() error {
	if strings.TrimSpace(p.SessionID) == "" {
		return errors.New("session deletion id is empty")
	}
	switch p.Phase {
	case SessionDeletionPrepared:
		if err := p.CommandID.Validate(); err != nil {
			return fmt.Errorf("session deletion command: %w", err)
		}
	case SessionDeletionConfirmed:
		if p.CommandID != "" {
			if err := p.CommandID.Validate(); err != nil {
				return fmt.Errorf("session deletion command: %w", err)
			}
		}
	default:
		return fmt.Errorf("session deletion phase %q is invalid", p.Phase)
	}
	if err := p.Replay.Validate(); err != nil {
		return err
	}
	return nil
}

// Request reconstructs the exact runtime mutation owned by a prepared record.
func (p PendingSessionDeletion) Request() agent.DeleteSession {
	return agent.DeleteSession{CommandID: p.CommandID, SessionID: p.SessionID}
}

type PendingRunState string

const (
	PendingRunQueued      PendingRunState = "queued"
	PendingRunDispatching PendingRunState = "dispatching"
	PendingRunCanceling   PendingRunState = "canceling"
)

// PendingRun is one durable runtime outbox entry. State distinguishes intent
// that has never left the queue from an ambiguous command handshake.
type PendingRun struct {
	State           PendingRunState `json:"state"`
	Command         agent.StartRun  `json:"command"`
	Replay          ReplayGuard     `json:"replay"`
	CancelCommandID agent.CommandID `json:"cancelCommandId,omitempty"`
	CancelReplay    ReplayGuard     `json:"cancelReplay"`
}

// PendingResume is a HITL decision whose command may already have reached the
// runtime. It remains durable until the runtime either acknowledges the exact
// command identity or definitively rejects it.
type PendingResume struct {
	Command      agent.ResumeRun     `json:"-"`
	Interactions []agent.Interaction `json:"interactions"`
	Replay       ReplayGuard         `json:"replay"`
}

func (p PendingResume) validate() error {
	if err := p.Command.Validate(); err != nil {
		return err
	}
	if err := p.Replay.Validate(); err != nil {
		return err
	}
	if p.Command.CommandID == "" {
		return errors.New("resume command id is empty")
	}
	if err := agent.ValidateInteractions(p.Interactions); err != nil {
		return err
	}
	for index, interaction := range p.Interactions {
		if agent.InteractionRunID(interaction) != p.Command.RunID {
			return fmt.Errorf("interaction %d belongs to another run", index+1)
		}
	}
	if len(p.Command.Answers) != len(p.Interactions) {
		return errors.New("resume answer count does not match interactions")
	}
	for index, interaction := range p.Interactions {
		response := p.Command.Answers[index]
		if response.ItemID != agent.InteractionItemID(interaction) {
			return fmt.Errorf("resume answer %d targets another interaction", index+1)
		}
		if err := agent.ValidateAnswer(interaction, response.Answer); err != nil {
			return fmt.Errorf("resume answer %d: %w", index+1, err)
		}
	}
	return nil
}

type pendingResumeJSON struct {
	CommandID    agent.CommandID          `json:"commandId"`
	RunID        string                   `json:"runId"`
	Message      *agent.Message           `json:"message,omitempty"`
	Interactions []pendingInteractionJSON `json:"interactions"`
	Replay       ReplayGuard              `json:"replay"`
}

type pendingInteractionJSON struct {
	Kind           string                `json:"kind"`
	Approval       *agent.Approval       `json:"approval,omitempty"`
	Question       *agent.Question       `json:"question,omitempty"`
	ApprovalAnswer *agent.ApprovalAnswer `json:"approvalAnswer,omitempty"`
	QuestionAnswer *agent.QuestionAnswer `json:"questionAnswer,omitempty"`
}

func (p PendingResume) MarshalJSON() ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	wire := pendingResumeJSON{
		CommandID:    p.Command.CommandID,
		RunID:        p.Command.RunID,
		Message:      p.Command.Message,
		Interactions: make([]pendingInteractionJSON, len(p.Interactions)),
		Replay:       p.Replay,
	}
	for index, interaction := range p.Interactions {
		answer := p.Command.Answers[index].Answer
		switch item := interaction.(type) {
		case agent.Approval:
			decision := answer.(agent.ApprovalAnswer)
			cloned := item.Clone()
			wire.Interactions[index] = pendingInteractionJSON{
				Kind: "approval", Approval: &cloned, ApprovalAnswer: &decision,
			}
		case agent.Question:
			response := agent.CloneAnswer(answer).(agent.QuestionAnswer)
			cloned := item.Clone()
			wire.Interactions[index] = pendingInteractionJSON{
				Kind: "question", Question: &cloned, QuestionAnswer: &response,
			}
		}
	}
	return json.Marshal(wire)
}

func (p *PendingResume) UnmarshalJSON(encoded []byte) error {
	var wire pendingResumeJSON
	if err := json.Unmarshal(encoded, &wire); err != nil {
		return err
	}
	decoded := PendingResume{
		Command: agent.ResumeRun{
			CommandID: wire.CommandID, RunID: wire.RunID, Message: wire.Message,
			Answers: make([]agent.InterruptAnswer, len(wire.Interactions)),
		},
		Interactions: make([]agent.Interaction, len(wire.Interactions)),
		Replay:       wire.Replay,
	}
	for index, item := range wire.Interactions {
		switch item.Kind {
		case "approval":
			if item.Approval == nil || item.ApprovalAnswer == nil || item.Question != nil || item.QuestionAnswer != nil {
				return fmt.Errorf("pending resume interaction %d has an invalid approval shape", index+1)
			}
			decoded.Interactions[index] = item.Approval.Clone()
			decoded.Command.Answers[index] = agent.InterruptAnswer{
				ItemID: item.Approval.ItemID, Answer: *item.ApprovalAnswer,
			}
		case "question":
			if item.Question == nil || item.QuestionAnswer == nil || item.Approval != nil || item.ApprovalAnswer != nil {
				return fmt.Errorf("pending resume interaction %d has an invalid question shape", index+1)
			}
			decoded.Interactions[index] = item.Question.Clone()
			decoded.Command.Answers[index] = agent.InterruptAnswer{
				ItemID: item.Question.ItemID, Answer: agent.CloneAnswer(*item.QuestionAnswer),
			}
		default:
			return fmt.Errorf("pending resume interaction %d has unknown kind %q", index+1, item.Kind)
		}
	}
	if err := decoded.validate(); err != nil {
		return err
	}
	*p = clonePendingResume(decoded)
	return nil
}

func (p PendingRun) validate(sessionID string) error {
	if p.State != PendingRunQueued && p.State != PendingRunDispatching && p.State != PendingRunCanceling {
		return fmt.Errorf("state %q is invalid", p.State)
	}
	if err := p.Command.Validate(); err != nil {
		return err
	}
	if p.Command.CommandID == "" {
		return errors.New("command id is empty")
	}
	if err := p.Replay.Validate(); err != nil {
		return err
	}
	if err := p.CancelReplay.Validate(); err != nil {
		return err
	}
	switch p.State {
	case PendingRunCanceling:
		if err := p.CancelCommandID.Validate(); err != nil {
			return fmt.Errorf("cancel command: %w", err)
		}
	default:
		if p.CancelCommandID != "" {
			return errors.New("non-canceling run carries a cancel command")
		}
	}
	if p.State == PendingRunQueued && (!p.Replay.Empty() || !p.CancelReplay.Empty()) {
		return errors.New("queued run carries a runtime replay guard")
	}
	if p.Command.SessionID != sessionID {
		return fmt.Errorf("command belongs to session %s", p.Command.SessionID)
	}
	return nil
}

func (p *PendingRun) beginDispatch(replay ReplayGuard) error {
	if err := replay.Validate(); err != nil {
		return err
	}
	switch p.State {
	case PendingRunQueued:
		p.State = PendingRunDispatching
		p.Replay = replay
		return nil
	case PendingRunDispatching, PendingRunCanceling:
		return nil
	default:
		return fmt.Errorf("pending run cannot begin dispatch from %q", p.State)
	}
}

func (p *PendingRun) beginCancellation(
	replay ReplayGuard,
	newCommandID func() (agent.CommandID, error),
) (agent.CommandID, error) {
	if err := replay.Validate(); err != nil {
		return "", err
	}
	switch p.State {
	case PendingRunDispatching:
		cancelCommandID, err := newCommandID()
		if err != nil {
			return "", err
		}
		p.State = PendingRunCanceling
		p.CancelCommandID = cancelCommandID
		p.CancelReplay = replay
		return cancelCommandID, nil
	case PendingRunCanceling:
		return p.CancelCommandID, nil
	default:
		return "", fmt.Errorf("pending run cannot begin cancellation from %q", p.State)
	}
}

func (p *PendingRun) requeue(newCommandID func() (agent.CommandID, error)) (agent.CommandID, error) {
	if p.State != PendingRunDispatching {
		return "", fmt.Errorf("pending run cannot be requeued from %q", p.State)
	}
	replacement, err := newCommandID()
	if err != nil {
		return "", err
	}
	p.State = PendingRunQueued
	p.Command.CommandID = replacement
	p.CancelCommandID = ""
	p.Replay = ReplayGuard{}
	p.CancelReplay = ReplayGuard{}
	return replacement, nil
}

func (p PendingRun) acknowledgeable() error {
	if p.State != PendingRunDispatching && p.State != PendingRunCanceling {
		return fmt.Errorf("pending run cannot be acknowledged from %q", p.State)
	}
	return nil
}

func validatePendingRunSequence(sessionID string, pending []PendingRun) error {
	seen := make(map[agent.CommandID]struct{}, len(pending))
	for index, command := range pending {
		if err := command.validate(sessionID); err != nil {
			return fmt.Errorf("pending run %d: %w", index+1, err)
		}
		if _, duplicate := seen[command.Command.CommandID]; duplicate {
			return fmt.Errorf("pending run %d repeats command %s", index+1, command.Command.CommandID)
		}
		seen[command.Command.CommandID] = struct{}{}
		if index > 0 && command.State != PendingRunQueued {
			return fmt.Errorf("pending run %d is %s behind the FIFO boundary", index+1, command.State)
		}
	}
	return nil
}

// Store is the aggregate root for CLI authoring state. Every mutating method
// updates memory only after its durable replacement succeeds.
type Store struct {
	mu               sync.Mutex
	directory        string
	historyLimit     int
	stashLimit       int
	workspaceLimit   int
	now              func() time.Time
	random           io.Reader
	history          []historyEntry
	drafts           map[string]agent.Message
	stashes          []Stash
	workspaces       []Workspace
	pendingRuns      map[string][]PendingRun
	pendingResumes   map[string]PendingResume
	pendingRollbacks map[string]PendingSessionRollback
	pendingSteers    map[string]PendingSteer
	sessionDeletions map[string]PendingSessionDeletion
	draftTransfer    *DraftTransfer
}

// Open loads a file-backed store. An empty directory creates a memory-only
// store, which keeps embedders and in-memory tests explicit.
func Open(directory string, config Config) (*Store, error) {
	store := &Store{
		directory:        strings.TrimSpace(directory),
		historyLimit:     positiveOr(config.HistoryLimit, defaultHistoryLimit),
		stashLimit:       positiveOr(config.StashLimit, defaultStashLimit),
		workspaceLimit:   positiveOr(config.WorkspaceLimit, defaultWorkspaceCap),
		now:              config.Now,
		random:           config.Random,
		drafts:           make(map[string]agent.Message),
		pendingRuns:      make(map[string][]PendingRun),
		pendingResumes:   make(map[string]PendingResume),
		pendingRollbacks: make(map[string]PendingSessionRollback),
		pendingSteers:    make(map[string]PendingSteer),
		sessionDeletions: make(map[string]PendingSessionDeletion),
	}
	if store.now == nil {
		store.now = time.Now
	}
	if store.random == nil {
		store.random = rand.Reader
	}
	if store.directory == "" {
		return store, nil
	}
	if !filepath.IsAbs(store.directory) {
		return nil, errors.New("workbench directory must be absolute")
	}
	store.directory = filepath.Clean(store.directory)
	if err := os.MkdirAll(store.directory, 0o700); err != nil {
		return nil, fmt.Errorf("create workbench directory: %w", err)
	}
	if err := store.loadOptional("history.json", &store.history); err != nil {
		return nil, fmt.Errorf("load prompt history: %w", err)
	}
	if err := validateHistory(store.history); err != nil {
		return nil, fmt.Errorf("load prompt history: %w", err)
	}
	if err := store.loadOptional("stashes.json", &store.stashes); err != nil {
		return nil, fmt.Errorf("load prompt stashes: %w", err)
	}
	if err := store.loadOptional("workspaces.json", &store.workspaces); err != nil {
		return nil, fmt.Errorf("load recent workspaces: %w", err)
	}
	var deletions []PendingSessionDeletion
	if err := store.loadOptional(sessionDeletionsName, &deletions); err != nil {
		return nil, fmt.Errorf("load session deletions: %w", err)
	}
	for index, pending := range deletions {
		pending.SessionID = strings.TrimSpace(pending.SessionID)
		if err := pending.validate(); err != nil {
			return nil, fmt.Errorf("load session deletion %d: %w", index+1, err)
		}
		if _, duplicate := store.sessionDeletions[pending.SessionID]; duplicate {
			return nil, fmt.Errorf("load session deletions: session %q appears more than once", pending.SessionID)
		}
		store.sessionDeletions[pending.SessionID] = pending
	}
	if err := store.loadSessionStates(); err != nil {
		return nil, fmt.Errorf("load session authoring state: %w", err)
	}
	var draftTransfer *DraftTransfer
	if err := store.loadOptional(sessionDraftTransferName, &draftTransfer); err != nil {
		return nil, fmt.Errorf("load session draft transfer: %w", err)
	}
	if draftTransfer != nil {
		cloned := draftTransfer.normalized()
		store.draftTransfer = &cloned
		if err := store.recoverDraftTransfer(); err != nil {
			return nil, fmt.Errorf("recover session draft transfer: %w", err)
		}
	}
	store.recoverConfirmedSessionDeletions()
	store.history = store.trimHistory(store.history)
	store.stashes = tailStashes(store.stashes, store.stashLimit)
	store.workspaces = slices.Clone(store.workspaces[:min(len(store.workspaces), store.workspaceLimit)])
	if err := store.recoverStashTransfer(); err != nil {
		return nil, fmt.Errorf("recover prompt stash transfer: %w", err)
	}
	return store, nil
}

// PendingRuns returns unacknowledged run-opening commands in authoring order.
func (s *Store) PendingRuns(sessionID string) []PendingRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePendingRunSlice(s.pendingRuns[strings.TrimSpace(sessionID)])
}

// PendingResume returns the unacknowledged HITL command for one session.
func (s *Store) PendingResume(sessionID string) (PendingResume, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pendingResumes[strings.TrimSpace(sessionID)]
	return clonePendingResume(pending), ok
}

// PendingSessionDeletions returns deletion journals in stable session order.
func (s *Store) PendingSessionDeletions() []PendingSessionDeletion {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sortedSessionDeletions(s.sessionDeletions)
}

// PendingSessionDeletion returns the journal for one session, when present.
func (s *Store) PendingSessionDeletion(sessionID string) (PendingSessionDeletion, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.sessionDeletions[strings.TrimSpace(sessionID)]
	return pending, ok
}

// StageSessionDeletion durably owns a runtime mutation before it can leave the
// process. Repeating the exact command is idempotent; another identity cannot
// replace an outcome whose acknowledgement is still unknown.
// StageSessionDeletion durably owns a deletion and the exact runtime
// store that may replay it after process restart.
func (s *Store) StageSessionDeletion(request agent.DeleteSession, replay ReplayGuard) error {
	request.SessionID = strings.TrimSpace(request.SessionID)
	if err := request.Validate(); err != nil {
		return err
	}
	if request.CommandID == "" {
		return errors.New("stage session deletion: command id is empty")
	}
	if err := replay.Validate(); err != nil {
		return err
	}
	pending := PendingSessionDeletion{
		Phase: SessionDeletionPrepared, CommandID: request.CommandID, SessionID: request.SessionID,
		Replay: replay,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draftTransferBlocks(request.SessionID) {
		return errors.New("session draft transfer requires recovery")
	}
	if current, exists := s.sessionDeletions[request.SessionID]; exists {
		if current == pending {
			return nil
		}
		return errors.New("another session deletion is already pending")
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	next[request.SessionID] = pending
	if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
		return err
	}
	s.sessionDeletions = next
	return nil
}

// RejectSessionDeletion retires a prepared journal after the runtime
// definitively reports that the session still exists and was not deleted.
func (s *Store) RejectSessionDeletion(sessionID string, commandID agent.CommandID) error {
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.sessionDeletions[sessionID]
	if !exists {
		return nil
	}
	if pending.Phase != SessionDeletionPrepared || pending.CommandID != commandID {
		return errors.New("session deletion journal does not match the rejected command")
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	delete(next, sessionID)
	if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
		return err
	}
	s.sessionDeletions = next
	return nil
}

// ConfirmSessionDeletion converts the exact prepared command into a durable
// local tombstone, then retires all CLI-owned state. Once the tombstone is
// durable, physical cleanup is best-effort: an undeletable old aggregate can
// no longer be observed and will be retried on the next Open.
func (s *Store) ConfirmSessionDeletion(sessionID string, commandID agent.CommandID) error {
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.sessionDeletions[sessionID]
	if !exists || pending.Phase != SessionDeletionPrepared || pending.CommandID != commandID {
		return errors.New("session deletion journal does not match the confirmed command")
	}
	return s.retireSessionStateLocked(sessionID, pending)
}

// ActivateSessionState establishes that the runtime once again owns this
// identity before authoring state is loaded for it. A confirmed deletion must
// finish removing its old aggregate and tombstone first, preventing a later
// import with the same session ID from inheriting obsolete local state.
func (s *Store) ActivateSessionState(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draftTransferBlocks(sessionID) {
		return errors.New("session draft transfer requires recovery")
	}
	pending, exists := s.sessionDeletions[sessionID]
	if !exists {
		return nil
	}
	if pending.Phase == SessionDeletionPrepared {
		return errors.New("session deletion acknowledgement is still pending")
	}
	if err := s.remove(s.sessionStateName(sessionID)); err != nil {
		return fmt.Errorf("remove retired session state: %w", err)
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	delete(next, sessionID)
	if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
		return fmt.Errorf("clear retired session tombstone: %w", err)
	}
	s.sessionDeletions = next
	return nil
}

// StagePendingResume transfers a completed interaction review into the durable
// command outbox before delivery starts.
func (s *Store) StagePendingResume(sessionID string, pending PendingResume) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	if err := pending.validate(); err != nil {
		return err
	}
	pending = clonePendingResume(pending)
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.pendingResumes[sessionID]; exists {
		if current.Command.CommandID == pending.Command.CommandID {
			if !pendingResumeEqual(current, pending) {
				return errors.New("pending resume command identity already owns another decision")
			}
			return nil
		}
		return errors.New("another resume command is already pending")
	}
	next := clonePendingResumes(s.pendingResumes)
	next[sessionID] = pending
	if err := s.saveSessionStateWithResume(sessionID, s.drafts[sessionID], s.pendingRuns[sessionID], &pending); err != nil {
		return err
	}
	s.pendingResumes = next
	return nil
}

// AcknowledgePendingResume retires exactly the command whose runtime response
// was observed. A stale callback cannot delete a newer interaction decision.
func (s *Store) AcknowledgePendingResume(sessionID string, commandID agent.CommandID) error {
	return s.retirePendingResume(sessionID, commandID)
}

// RejectPendingResume releases a command after a definitive runtime refusal so
// its review can be edited and submitted under a fresh identity.
func (s *Store) RejectPendingResume(sessionID string, commandID agent.CommandID) error {
	return s.retirePendingResume(sessionID, commandID)
}

// RequeuePendingResume atomically gives a decision a fresh runtime identity
// after the owning store's authoritative waiting projection proves the old
// command did not commit before its replay guarantee expired.
func (s *Store) RequeuePendingResume(
	sessionID string,
	commandID agent.CommandID,
	replay ReplayGuard,
) (PendingResume, error) {
	if err := commandID.Validate(); err != nil {
		return PendingResume{}, err
	}
	if err := replay.Validate(); err != nil {
		return PendingResume{}, err
	}
	replacement, err := agent.NewCommandID()
	if err != nil {
		return PendingResume{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingResumes[sessionID]
	if !exists || pending.Command.CommandID != commandID {
		return PendingResume{}, errors.New("pending resume command identity changed")
	}
	pending = clonePendingResume(pending)
	pending.Command.CommandID = replacement
	pending.Replay = replay
	if err := pending.validate(); err != nil {
		return PendingResume{}, err
	}
	if err := s.saveSessionStateWithResume(
		sessionID, s.drafts[sessionID], s.pendingRuns[sessionID], &pending,
	); err != nil {
		return PendingResume{}, err
	}
	s.pendingResumes[sessionID] = pending
	return clonePendingResume(pending), nil
}

// DiscardPendingResume retires terminal authoring state for a session that the
// runtime has deleted or replaced. It never runs as part of ordinary session
// navigation, where the outstanding command must remain recoverable.
func (s *Store) DiscardPendingResume(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.pendingResumes[sessionID]; !exists {
		return nil
	}
	if err := s.saveSessionStateWithResume(sessionID, s.drafts[sessionID], s.pendingRuns[sessionID], nil); err != nil {
		return err
	}
	delete(s.pendingResumes, sessionID)
	return nil
}

func (s *Store) retirePendingResume(sessionID string, commandID agent.CommandID) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingResumes[sessionID]
	if !exists || pending.Command.CommandID != commandID {
		return errors.New("pending resume command identity changed")
	}
	if err := s.saveSessionStateWithResume(sessionID, s.drafts[sessionID], s.pendingRuns[sessionID], nil); err != nil {
		return err
	}
	delete(s.pendingResumes, sessionID)
	return nil
}

// StagePendingRun atomically moves one draft into the durable runtime outbox.
// A crash observes either the editable draft or the replayable command, never
// an ownership gap between separate files.
func (s *Store) StagePendingRun(pending PendingRun) error {
	if pending.State != PendingRunQueued {
		return fmt.Errorf("stage pending run: initial state must be %q", PendingRunQueued)
	}
	if err := pending.validate(pending.Command.SessionID); err != nil {
		return err
	}
	pending = clonePendingRun(pending)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, current := range s.pendingRuns[pending.Command.SessionID] {
		if current.Command.CommandID == pending.Command.CommandID {
			if !pendingRunEqual(current, pending) {
				return errors.New("pending run command identity already owns another payload")
			}
			return nil
		}
	}
	next := clonePendingRuns(s.pendingRuns)
	next[pending.Command.SessionID] = append(next[pending.Command.SessionID], pending)
	if err := validatePendingRunSequence(pending.Command.SessionID, next[pending.Command.SessionID]); err != nil {
		return err
	}
	if err := s.saveSessionState(pending.Command.SessionID, agent.Message{}, next[pending.Command.SessionID]); err != nil {
		return err
	}
	delete(s.drafts, pending.Command.SessionID)
	s.pendingRuns = next
	return nil
}

// SavePendingRuns atomically replaces one session's ordered runtime outbox.
// Queue edits use this boundary so reordering, replacement, and deletion are
// crash-consistent with the next launch.
func (s *Store) SavePendingRuns(sessionID string, commands []PendingRun) error {
	sessionID = strings.TrimSpace(sessionID)
	if err := validatePendingRunSequence(sessionID, commands); err != nil {
		return err
	}
	commands = clonePendingRunSlice(commands)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], commands); err != nil {
		return err
	}
	if len(commands) == 0 {
		delete(s.pendingRuns, sessionID)
	} else {
		s.pendingRuns[sessionID] = commands
	}
	return nil
}

func (s *Store) MarkPendingRunDispatching(
	sessionID string,
	commandID agent.CommandID,
	replay ReplayGuard,
) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	index := pendingRunIndex(s.pendingRuns[sessionID], commandID)
	if index < 0 {
		return errors.New("pending run is absent")
	}
	next := clonePendingRuns(s.pendingRuns)
	if err := next[sessionID][index].beginDispatch(replay); err != nil {
		return err
	}
	if next[sessionID][index].State == s.pendingRuns[sessionID][index].State {
		return nil
	}
	if err := validatePendingRunSequence(sessionID, next[sessionID]); err != nil {
		return err
	}
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], next[sessionID]); err != nil {
		return err
	}
	s.pendingRuns = next
	return nil
}

// MarkPendingRunCanceling durably records that a command with an uncertain
// acknowledgement must be canceled as soon as its run identity is recovered.
func (s *Store) MarkPendingRunCanceling(
	sessionID string,
	commandID agent.CommandID,
	replay ReplayGuard,
) (agent.CommandID, error) {
	if err := commandID.Validate(); err != nil {
		return "", err
	}
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	index := pendingRunIndex(s.pendingRuns[sessionID], commandID)
	if index < 0 {
		return "", errors.New("pending run is absent")
	}
	if s.pendingRuns[sessionID][index].State == PendingRunCanceling {
		return s.pendingRuns[sessionID][index].CancelCommandID, nil
	}
	next := clonePendingRuns(s.pendingRuns)
	cancelCommandID, err := next[sessionID][index].beginCancellation(replay, agent.NewCommandID)
	if err != nil {
		return "", err
	}
	if err := validatePendingRunSequence(sessionID, next[sessionID]); err != nil {
		return "", err
	}
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], next[sessionID]); err != nil {
		return "", err
	}
	s.pendingRuns = next
	return cancelCommandID, nil
}

// RequeuePendingRun turns a definitively refused runtime mutation back into an
// ordinary FIFO entry. A new identity is mandatory: the runtime has already
// bound the old key to its rejection outcome.
func (s *Store) RequeuePendingRun(sessionID string, commandID agent.CommandID) (agent.CommandID, error) {
	if err := commandID.Validate(); err != nil {
		return "", err
	}
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	index := pendingRunIndex(s.pendingRuns[sessionID], commandID)
	if index < 0 {
		return "", errors.New("pending run is absent")
	}
	next := clonePendingRuns(s.pendingRuns)
	replacement, err := next[sessionID][index].requeue(agent.NewCommandID)
	if err != nil {
		return "", err
	}
	if err := validatePendingRunSequence(sessionID, next[sessionID]); err != nil {
		return "", err
	}
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], next[sessionID]); err != nil {
		return "", err
	}
	s.pendingRuns = next
	return replacement, nil
}

// AcknowledgePendingRun retires only the command the caller actually observed.
// The identity check prevents a late acknowledgement from deleting newer work.
func (s *Store) AcknowledgePendingRun(sessionID string, commandID agent.CommandID) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	commands := s.pendingRuns[sessionID]
	index := pendingRunIndex(commands, commandID)
	if index < 0 {
		return errors.New("pending run command identity changed")
	}
	if err := commands[index].acknowledgeable(); err != nil {
		return err
	}
	message := commands[index].Command.Message.Clone()
	nextHistory := cloneHistory(s.history)
	historyIndex := slices.IndexFunc(nextHistory, func(entry historyEntry) bool {
		return entry.CommandID == commandID
	})
	if historyIndex >= 0 && !nextHistory[historyIndex].Equal(message) {
		return errors.New("prompt history command identity already owns another message")
	}
	if historyIndex < 0 {
		nextHistory = s.trimHistory(append(nextHistory, historyEntry{Message: message, CommandID: commandID}))
		if err := s.save("history.json", nextHistory); err != nil {
			return err
		}
		// History and the session outbox are separate durable aggregates. Publish
		// the completed first half immediately so a failed outbox replacement can
		// retry by command identity without appending the prompt a second time.
		s.history = nextHistory
	}
	next := clonePendingRuns(s.pendingRuns)
	next[sessionID] = slices.Delete(next[sessionID], index, index+1)
	if len(next[sessionID]) == 0 {
		delete(next, sessionID)
	}
	if err := validatePendingRunSequence(sessionID, next[sessionID]); err != nil {
		return err
	}
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], next[sessionID]); err != nil {
		return err
	}
	s.pendingRuns = next
	s.history = s.trimHistory(s.history)
	return nil
}

// History returns detached prompts in oldest-to-newest order.
func (s *Store) History() []agent.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := make([]agent.Message, len(s.history))
	for index, entry := range s.history {
		messages[index] = entry.Clone()
	}
	return messages
}

// Remember records a submitted or deliberately cleared prompt.
func (s *Store) Remember(message agent.Message) error {
	message = message.Clone()
	if messageEmpty(message) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) > 0 && s.history[len(s.history)-1].Equal(message) {
		return nil
	}
	next := append(cloneHistory(s.history), historyEntry{Message: message})
	next = s.trimHistory(next)
	if err := s.save("history.json", next); err != nil {
		return err
	}
	s.history = next
	return nil
}

// Draft loads a session-specific prompt without consuming it.
func (s *Store) Draft(sessionID string) (agent.Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	draft := s.drafts[strings.TrimSpace(sessionID)]
	return draft.Clone(), !messageEmpty(draft), nil
}

// SaveDraft atomically replaces a session draft, or removes it when empty.
func (s *Store) SaveDraft(sessionID string, message agent.Message) error {
	sessionID = strings.TrimSpace(sessionID)
	message = message.Clone()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, present := s.drafts[sessionID]
	if (present && current.Equal(message)) || (!present && messageEmpty(message)) {
		return nil
	}
	if err := s.saveSessionState(sessionID, message, s.pendingRuns[sessionID]); err != nil {
		return err
	}
	if messageEmpty(message) {
		delete(s.drafts, sessionID)
	} else {
		s.drafts[sessionID] = message
	}
	return nil
}

// DiscardDraft retires authoring state for a session that no longer exists.
// It is intentionally distinct from saving an empty draft at call sites: the
// caller is expressing a lifecycle transition, not an editor value change.
func (s *Store) DiscardDraft(sessionID string) error {
	return s.SaveDraft(sessionID, agent.Message{})
}

// RetireSessionState atomically removes every authoring concern owned by one
// session: its draft, pending run commands, pending HITL and steer commands,
// and rollback journal. Session deletion cannot safely compose the narrower
// mutations because each rewrites the same durable aggregate. A failure between
// them would expose a partially retired session after restart.
func (s *Store) RetireSessionState(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := s.sessionDeletions[sessionID]
	return s.retireSessionStateLocked(sessionID, pending)
}

func (s *Store) retireSessionStateLocked(sessionID string, pending PendingSessionDeletion) error {
	if s.draftTransferBlocks(sessionID) {
		return errors.New("session draft transfer requires recovery")
	}
	confirmed := PendingSessionDeletion{
		Phase: SessionDeletionConfirmed, CommandID: pending.CommandID, SessionID: sessionID,
	}
	if pending.Phase != SessionDeletionConfirmed {
		next := cloneSessionDeletions(s.sessionDeletions)
		next[sessionID] = confirmed
		if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
			return fmt.Errorf("save retired session tombstone: %w", err)
		}
		s.sessionDeletions = next
	}
	delete(s.drafts, sessionID)
	delete(s.pendingRuns, sessionID)
	delete(s.pendingResumes, sessionID)
	delete(s.pendingRollbacks, sessionID)
	delete(s.pendingSteers, sessionID)
	if err := s.remove(s.sessionStateName(sessionID)); err != nil {
		return nil
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	delete(next, sessionID)
	if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
		return nil
	}
	s.sessionDeletions = next
	return nil
}

// StashPrompt preserves a prompt independently of its session draft.
func (s *Store) StashPrompt(message agent.Message) (Stash, error) {
	message = message.Clone()
	if messageEmpty(message) {
		return Stash{}, errors.New("cannot stash an empty prompt")
	}
	identity := make([]byte, 8)
	if _, err := io.ReadFull(s.random, identity); err != nil {
		return Stash{}, fmt.Errorf("create stash id: %w", err)
	}
	stash := Stash{ID: hex.EncodeToString(identity), CreatedAt: s.now().UTC(), Message: message}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := append(slices.Clone(s.stashes), stash)
	next = tailStashes(next, s.stashLimit)
	if err := s.save("stashes.json", next); err != nil {
		return Stash{}, err
	}
	s.stashes = next
	return cloneStash(stash), nil
}

// StashDraft transfers one session draft into the bounded stash collection.
// A durable intent makes the cross-file move restart-safe; synchronous failure
// restores the complete pre-transaction stash collection so capacity eviction
// cannot turn compensation into data loss.
func (s *Store) StashDraft(sessionID string, message agent.Message) (Stash, error) {
	sessionID = strings.TrimSpace(sessionID)
	message = message.Clone()
	if sessionID == "" {
		return Stash{}, errors.New("session id is empty")
	}
	if messageEmpty(message) {
		return Stash{}, errors.New("cannot stash an empty prompt")
	}
	identity := make([]byte, 8)
	if _, err := io.ReadFull(s.random, identity); err != nil {
		return Stash{}, fmt.Errorf("create stash id: %w", err)
	}
	stash := Stash{ID: hex.EncodeToString(identity), CreatedAt: s.now().UTC(), Message: message}
	transfer := stashTransfer{SessionID: sessionID, Draft: message, Stash: stash}

	s.mu.Lock()
	defer s.mu.Unlock()
	current, present := s.drafts[sessionID]
	if !present || !current.Equal(message) {
		return Stash{}, errors.New("session draft changed before it could be stashed")
	}
	previous := slices.Clone(s.stashes)
	next := tailStashes(append(slices.Clone(s.stashes), stash), s.stashLimit)
	if err := s.save(stashTransferName, transfer); err != nil {
		return Stash{}, fmt.Errorf("save stash transfer: %w", err)
	}
	if err := s.save("stashes.json", next); err != nil {
		_ = s.remove(stashTransferName)
		return Stash{}, err
	}
	if err := s.saveSessionState(sessionID, agent.Message{}, s.pendingRuns[sessionID]); err != nil {
		rollbackErr := s.save("stashes.json", previous)
		if rollbackErr != nil {
			s.stashes = next
			return Stash{}, errors.Join(
				fmt.Errorf("clear session draft: %w", err),
				fmt.Errorf("restore prompt stashes: %w", rollbackErr),
			)
		}
		if cleanupErr := s.remove(stashTransferName); cleanupErr != nil {
			// Re-publish the intended stash so a surviving journal always describes
			// a forward-recoverable state rather than reviving a rolled-back move.
			if restoreErr := s.save("stashes.json", next); restoreErr != nil {
				return Stash{}, errors.Join(
					fmt.Errorf("clear session draft: %w", err),
					fmt.Errorf("remove stash transfer: %w", cleanupErr),
					fmt.Errorf("restore recoverable stash: %w", restoreErr),
				)
			}
			s.stashes = next
			return Stash{}, errors.Join(
				fmt.Errorf("clear session draft: %w", err),
				fmt.Errorf("remove stash transfer: %w", cleanupErr),
			)
		}
		return Stash{}, fmt.Errorf("clear session draft: %w", err)
	}
	s.stashes = next
	delete(s.drafts, sessionID)
	// Once the source draft is absent, the transfer is committed. A stale
	// journal is harmless: recovery sees no matching source owner and only
	// retries this cleanup.
	_ = s.remove(stashTransferName)
	return cloneStash(stash), nil
}

func (s *Store) recoverStashTransfer() error {
	var transfer stashTransfer
	if err := s.loadOptional(stashTransferName, &transfer); err != nil {
		return err
	}
	if transfer.SessionID == "" {
		return nil
	}
	if err := transfer.validate(); err != nil {
		return err
	}
	current, present := s.drafts[transfer.SessionID]
	if present && current.Equal(transfer.Draft) {
		index := slices.IndexFunc(s.stashes, func(stash Stash) bool { return stash.ID == transfer.Stash.ID })
		switch {
		case index >= 0 && !stashEqual(s.stashes[index], transfer.Stash):
			return errors.New("stash transfer identity belongs to another prompt")
		case index < 0:
			next := tailStashes(append(slices.Clone(s.stashes), transfer.Stash), s.stashLimit)
			if err := s.save("stashes.json", next); err != nil {
				return fmt.Errorf("save recovered prompt stash: %w", err)
			}
			s.stashes = next
		}
		if err := s.saveSessionState(transfer.SessionID, agent.Message{}, s.pendingRuns[transfer.SessionID]); err != nil {
			return fmt.Errorf("retire recovered session draft: %w", err)
		}
		delete(s.drafts, transfer.SessionID)
	}
	// The move is already complete when the source is absent, and a newer draft
	// means the old intent no longer owns that session value. Either state makes
	// replay unnecessary; cleanup is best-effort because the journal is
	// idempotent under both conditions.
	_ = s.remove(stashTransferName)
	return nil
}

// Stashes returns newest prompts first.
func (s *Store) Stashes() []Stash {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Stash, len(s.stashes))
	for i, stash := range slices.Backward(s.stashes) {
		out[len(s.stashes)-1-i] = cloneStash(stash)
	}
	return out
}

// Stash returns one detached prompt by identity.
func (s *Store) Stash(id string) (Stash, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, stash := range s.stashes {
		if stash.ID == id {
			return cloneStash(stash), true
		}
	}
	return Stash{}, false
}

// DeleteStash permanently removes one stash.
func (s *Store) DeleteStash(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.DeleteFunc(slices.Clone(s.stashes), func(stash Stash) bool { return stash.ID == id })
	if len(next) == len(s.stashes) {
		return false, nil
	}
	if err := s.save("stashes.json", next); err != nil {
		return false, err
	}
	s.stashes = next
	return true, nil
}

// RememberWorkspace moves a workspace to the front of the recent list.
func (s *Store) RememberWorkspace(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || !filepath.IsAbs(path) {
		return errors.New("workspace path must be absolute")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.DeleteFunc(slices.Clone(s.workspaces), func(item Workspace) bool { return item.Path == path })
	next = slices.Insert(next, 0, Workspace{Path: path, LastOpened: s.now().UTC()})
	next = next[:min(len(next), s.workspaceLimit)]
	if err := s.save("workspaces.json", next); err != nil {
		return err
	}
	s.workspaces = next
	return nil
}

// Workspaces returns recent workspaces in newest-first order.
func (s *Store) Workspaces() []Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.workspaces)
}

type envelope[T any] struct {
	Version int `json:"version"`
	Value   T   `json:"value"`
}

func (s *Store) load(name string, value any) error {
	if s.directory == "" {
		return os.ErrNotExist
	}
	file, err := os.Open(s.path(name))
	if err != nil {
		return err
	}
	defer file.Close()
	var raw envelope[json.RawMessage]
	decoder := json.NewDecoder(io.LimitReader(file, 16<<20))
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if raw.Version != formatVersion {
		return fmt.Errorf("unsupported workbench format %d", raw.Version)
	}
	if err := json.Unmarshal(raw.Value, value); err != nil {
		return err
	}
	return nil
}

func (s *Store) loadOptional(name string, value any) error {
	err := s.load(name, value)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) loadSessionStates() error {
	if s.directory == "" {
		return nil
	}
	directory := s.path("sessions")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if s.confirmedSessionStateFile(entry.Name()) {
			continue
		}
		var state sessionState
		if err := s.load(filepath.Join("sessions", entry.Name()), &state); err != nil {
			return fmt.Errorf("load %s: %w", entry.Name(), err)
		}
		state.SessionID = strings.TrimSpace(state.SessionID)
		if state.SessionID == "" || entry.Name() != filepath.Base(s.sessionStateName(state.SessionID)) {
			return fmt.Errorf("state %s has an invalid session identity", entry.Name())
		}
		if err := validatePendingRunSequence(state.SessionID, state.PendingRuns); err != nil {
			return fmt.Errorf("state %s: %w", entry.Name(), err)
		}
		if !messageEmpty(state.Draft) {
			s.drafts[state.SessionID] = state.Draft.Clone()
		}
		if len(state.PendingRuns) > 0 {
			s.pendingRuns[state.SessionID] = clonePendingRunSlice(state.PendingRuns)
		}
		if state.PendingResume != nil {
			if err := state.PendingResume.validate(); err != nil {
				return fmt.Errorf("state %s pending resume: %w", entry.Name(), err)
			}
			s.pendingResumes[state.SessionID] = clonePendingResume(*state.PendingResume)
		}
		if state.PendingRollback != nil {
			pending := state.PendingRollback.clone()
			if err := pending.Validate(); err != nil {
				return fmt.Errorf("state %s pending rollback: %w", entry.Name(), err)
			}
			if pending.SessionID != state.SessionID {
				return fmt.Errorf("state %s pending rollback belongs to another session", entry.Name())
			}
			s.pendingRollbacks[state.SessionID] = pending
		}
		if state.PendingSteer != nil {
			pending := state.PendingSteer.clone()
			if err := pending.validate(state.SessionID); err != nil {
				return fmt.Errorf("state %s pending steer: %w", entry.Name(), err)
			}
			s.pendingSteers[state.SessionID] = pending
		}
	}
	return nil
}

func (s *Store) confirmedSessionStateFile(name string) bool {
	for sessionID, pending := range s.sessionDeletions {
		if pending.Phase == SessionDeletionConfirmed && filepath.Base(s.sessionStateName(sessionID)) == name {
			return true
		}
	}
	return false
}

// recoverConfirmedSessionDeletions performs cleanup which is no longer on the
// correctness path. Failures deliberately leave the tombstone in place; the
// obsolete aggregate remains unreachable and a later Open retries it.
func (s *Store) recoverConfirmedSessionDeletions() {
	for _, pending := range sortedSessionDeletions(s.sessionDeletions) {
		if pending.Phase != SessionDeletionConfirmed {
			continue
		}
		if err := s.remove(s.sessionStateName(pending.SessionID)); err != nil {
			continue
		}
		next := cloneSessionDeletions(s.sessionDeletions)
		delete(next, pending.SessionID)
		if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
			continue
		}
		s.sessionDeletions = next
	}
}

func (s *Store) saveSessionState(sessionID string, draft agent.Message, pending []PendingRun) error {
	resume, ok := s.pendingResumes[sessionID]
	if !ok {
		return s.saveSessionStateWithResume(sessionID, draft, pending, nil)
	}
	return s.saveSessionStateWithResume(sessionID, draft, pending, &resume)
}

func (s *Store) saveSessionStateWithResume(
	sessionID string,
	draft agent.Message,
	pending []PendingRun,
	resume *PendingResume,
) error {
	sessionID = strings.TrimSpace(sessionID)
	var rollback *PendingSessionRollback
	if pendingRollback, exists := s.pendingRollbacks[sessionID]; exists {
		cloned := pendingRollback.clone()
		rollback = &cloned
	}
	var steer *PendingSteer
	if pendingSteer, exists := s.pendingSteers[sessionID]; exists {
		cloned := pendingSteer.clone()
		steer = &cloned
	}
	return s.saveSessionStateRecord(sessionID, draft, pending, resume, rollback, steer)
}

func (s *Store) saveSessionStateRecord(
	sessionID string,
	draft agent.Message,
	pending []PendingRun,
	resume *PendingResume,
	rollback *PendingSessionRollback,
	steer *PendingSteer,
) error {
	if s.draftTransferBlocks(sessionID) {
		return errors.New("session draft transfer requires recovery")
	}
	return s.saveSessionStateRecordUnfenced(sessionID, draft, pending, resume, rollback, steer)
}

func (s *Store) saveSessionStateRecordUnfenced(
	sessionID string,
	draft agent.Message,
	pending []PendingRun,
	resume *PendingResume,
	rollback *PendingSessionRollback,
	steer *PendingSteer,
) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	if pending, exists := s.sessionDeletions[sessionID]; exists && pending.Phase == SessionDeletionConfirmed {
		return errors.New("session authoring state has been retired")
	}
	name := s.sessionStateName(sessionID)
	if messageEmpty(draft) && len(pending) == 0 && resume == nil && rollback == nil && steer == nil {
		if s.directory == "" {
			return nil
		}
		err := os.Remove(s.path(name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	state := sessionState{
		SessionID: sessionID, Draft: draft.Clone(), PendingRuns: clonePendingRunSlice(pending),
	}
	if resume != nil {
		cloned := clonePendingResume(*resume)
		state.PendingResume = &cloned
	}
	if rollback != nil {
		cloned := rollback.clone()
		state.PendingRollback = &cloned
	}
	if steer != nil {
		cloned := steer.clone()
		state.PendingSteer = &cloned
	}
	return s.save(name, state)
}

func (s *Store) save(name string, value any) error {
	if s.directory == "" {
		return nil
	}
	path := s.path(name)
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".lyra-state-*")
	if err != nil {
		return fmt.Errorf("create state snapshot: %w", err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(envelope[any]{Version: formatVersion, Value: value}); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode state snapshot: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync state snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close state snapshot: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace state snapshot: %w", err)
	}
	removeTemporary = false
	return nil
}

func (s *Store) remove(name string) error {
	if s.directory == "" {
		return nil
	}
	err := os.Remove(s.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) path(name string) string { return pathologize.Join(s.directory, name) }

func (s *Store) sessionStateName(sessionID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(sessionID)))
	return filepath.Join("sessions", hex.EncodeToString(digest[:16])+".json")
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func validateHistory(history []historyEntry) error {
	seen := make(map[agent.CommandID]struct{}, len(history))
	for index, entry := range history {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("entry %d: %w", index+1, err)
		}
		if entry.CommandID == "" {
			continue
		}
		if err := entry.CommandID.Validate(); err != nil {
			return fmt.Errorf("entry %d command: %w", index+1, err)
		}
		if _, duplicate := seen[entry.CommandID]; duplicate {
			return fmt.Errorf("entry %d repeats command %s", index+1, entry.CommandID)
		}
		seen[entry.CommandID] = struct{}{}
	}
	return nil
}

func (s *Store) trimHistory(history []historyEntry) []historyEntry {
	if len(history) <= s.historyLimit {
		return cloneHistory(history)
	}
	pinned := make(map[agent.CommandID]struct{})
	for _, commands := range s.pendingRuns {
		for _, pending := range commands {
			pinned[pending.Command.CommandID] = struct{}{}
		}
	}
	pinnedHistory := 0
	for _, entry := range history {
		if _, protected := pinned[entry.CommandID]; protected {
			pinnedHistory++
		}
	}
	nonPinnedBudget := s.historyLimit
	keepNonPinned := make(map[int]struct{}, nonPinnedBudget)
	for index := len(history) - 1; index >= 0 && len(keepNonPinned) < nonPinnedBudget; index-- {
		if _, protected := pinned[history[index].CommandID]; !protected {
			keepNonPinned[index] = struct{}{}
		}
	}
	trimmed := make([]historyEntry, 0, min(len(history), s.historyLimit+pinnedHistory))
	for index, entry := range history {
		_, protected := pinned[entry.CommandID]
		_, recent := keepNonPinned[index]
		if protected || recent {
			trimmed = append(trimmed, historyEntry{Message: entry.Clone(), CommandID: entry.CommandID})
		}
	}
	return trimmed
}

func tailStashes(stashes []Stash, limit int) []Stash {
	if len(stashes) > limit {
		stashes = stashes[len(stashes)-limit:]
	}
	out := make([]Stash, len(stashes))
	for i, stash := range stashes {
		out[i] = cloneStash(stash)
	}
	return out
}

func cloneHistory(history []historyEntry) []historyEntry {
	out := make([]historyEntry, len(history))
	for index, entry := range history {
		out[index] = historyEntry{Message: entry.Clone(), CommandID: entry.CommandID}
	}
	return out
}

func clonePendingRuns(pending map[string][]PendingRun) map[string][]PendingRun {
	out := make(map[string][]PendingRun, len(pending))
	for sessionID, commands := range pending {
		out[sessionID] = clonePendingRunSlice(commands)
	}
	return out
}

func clonePendingResumes(pending map[string]PendingResume) map[string]PendingResume {
	out := make(map[string]PendingResume, len(pending))
	for sessionID, command := range pending {
		out[sessionID] = clonePendingResume(command)
	}
	return out
}

func clonePendingRunSlice(commands []PendingRun) []PendingRun {
	out := make([]PendingRun, len(commands))
	for index, command := range commands {
		out[index] = clonePendingRun(command)
	}
	return out
}

func clonePendingRun(command PendingRun) PendingRun {
	command.Command = command.Command.Clone()
	return command
}

func pendingRunEqual(left, right PendingRun) bool {
	return left.State == right.State && left.Replay == right.Replay &&
		left.CancelCommandID == right.CancelCommandID && left.CancelReplay == right.CancelReplay &&
		left.Command.Equal(right.Command)
}

func pendingResumeEqual(left, right PendingResume) bool {
	return left.Command.Equal(right.Command) && left.Replay == right.Replay &&
		agent.InteractionsEqual(left.Interactions, right.Interactions)
}

func clonePendingResume(pending PendingResume) PendingResume {
	pending.Command = pending.Command.Clone()
	pending.Interactions = agent.CloneInteractions(pending.Interactions)
	return pending
}

func pendingRunIndex(commands []PendingRun, commandID agent.CommandID) int {
	return slices.IndexFunc(commands, func(command PendingRun) bool { return command.Command.CommandID == commandID })
}

func cloneStash(stash Stash) Stash {
	stash.Message = stash.Message.Clone()
	return stash
}

func cloneSessionDeletions(source map[string]PendingSessionDeletion) map[string]PendingSessionDeletion {
	cloned := make(map[string]PendingSessionDeletion, len(source))
	for sessionID, pending := range source {
		cloned[sessionID] = pending
	}
	return cloned
}

func sortedSessionDeletions(source map[string]PendingSessionDeletion) []PendingSessionDeletion {
	deletions := make([]PendingSessionDeletion, 0, len(source))
	for _, pending := range source {
		deletions = append(deletions, pending)
	}
	slices.SortFunc(deletions, func(left, right PendingSessionDeletion) int {
		return strings.Compare(left.SessionID, right.SessionID)
	})
	return deletions
}

func messageEmpty(message agent.Message) bool {
	return strings.TrimSpace(message.Text) == "" && len(message.Attachments) == 0
}
