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
	formatVersion       = 1
	defaultHistoryLimit = 1000
	defaultStashLimit   = 100
	defaultWorkspaceCap = 50
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

// Workspace is one recently used authoring root.
type Workspace struct {
	Path       string    `json:"path"`
	LastOpened time.Time `json:"lastOpened"`
}

type sessionState struct {
	SessionID     string         `json:"sessionId"`
	Draft         agent.Message  `json:"draft"`
	PendingRuns   []PendingRun   `json:"pendingRuns"`
	PendingResume *PendingResume `json:"pendingResume,omitempty"`
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
	CancelCommandID agent.CommandID `json:"cancelCommandId,omitempty"`
}

// PendingResume is a HITL decision whose command may already have reached the
// runtime. It remains durable until the runtime either acknowledges the exact
// command identity or definitively rejects it.
type PendingResume struct {
	Command      agent.ResumeRun     `json:"-"`
	Interactions []agent.Interaction `json:"interactions"`
}

func (pending PendingResume) validate() error {
	if err := pending.Command.Validate(); err != nil {
		return err
	}
	if err := agent.ValidateInteractions(pending.Interactions); err != nil {
		return err
	}
	for index, interaction := range pending.Interactions {
		if agent.InteractionRunID(interaction) != pending.Command.RunID {
			return fmt.Errorf("interaction %d belongs to another run", index+1)
		}
	}
	if len(pending.Command.Answers) != len(pending.Interactions) {
		return errors.New("resume answer count does not match interactions")
	}
	for index, interaction := range pending.Interactions {
		response := pending.Command.Answers[index]
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
}

type pendingInteractionJSON struct {
	Kind           string                `json:"kind"`
	Approval       *agent.Approval       `json:"approval,omitempty"`
	Question       *agent.Question       `json:"question,omitempty"`
	ApprovalAnswer *agent.ApprovalAnswer `json:"approvalAnswer,omitempty"`
	QuestionAnswer *agent.QuestionAnswer `json:"questionAnswer,omitempty"`
}

func (pending PendingResume) MarshalJSON() ([]byte, error) {
	if err := pending.validate(); err != nil {
		return nil, err
	}
	wire := pendingResumeJSON{
		CommandID:    pending.Command.CommandID,
		RunID:        pending.Command.RunID,
		Message:      pending.Command.Message,
		Interactions: make([]pendingInteractionJSON, len(pending.Interactions)),
	}
	for index, interaction := range pending.Interactions {
		answer := pending.Command.Answers[index].Answer
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

func (pending *PendingResume) UnmarshalJSON(encoded []byte) error {
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
	*pending = clonePendingResume(decoded)
	return nil
}

func (pending PendingRun) validate(sessionID string) error {
	if pending.State != PendingRunQueued && pending.State != PendingRunDispatching && pending.State != PendingRunCanceling {
		return fmt.Errorf("state %q is invalid", pending.State)
	}
	if err := pending.Command.Validate(); err != nil {
		return err
	}
	if pending.Command.CommandID == "" {
		return errors.New("command id is empty")
	}
	switch pending.State {
	case PendingRunCanceling:
		if err := pending.CancelCommandID.Validate(); err != nil {
			return fmt.Errorf("cancel command: %w", err)
		}
	default:
		if pending.CancelCommandID != "" {
			return errors.New("non-canceling run carries a cancel command")
		}
	}
	if pending.Command.SessionID != sessionID {
		return fmt.Errorf("command belongs to session %s", pending.Command.SessionID)
	}
	return nil
}

func (pending *PendingRun) beginDispatch() error {
	switch pending.State {
	case PendingRunQueued:
		pending.State = PendingRunDispatching
		return nil
	case PendingRunDispatching, PendingRunCanceling:
		return nil
	default:
		return fmt.Errorf("pending run cannot begin dispatch from %q", pending.State)
	}
}

func (pending *PendingRun) beginCancellation(newCommandID func() (agent.CommandID, error)) (agent.CommandID, error) {
	switch pending.State {
	case PendingRunDispatching:
		cancelCommandID, err := newCommandID()
		if err != nil {
			return "", err
		}
		pending.State = PendingRunCanceling
		pending.CancelCommandID = cancelCommandID
		return cancelCommandID, nil
	case PendingRunCanceling:
		return pending.CancelCommandID, nil
	default:
		return "", fmt.Errorf("pending run cannot begin cancellation from %q", pending.State)
	}
}

func (pending *PendingRun) requeue(newCommandID func() (agent.CommandID, error)) (agent.CommandID, error) {
	if pending.State != PendingRunDispatching {
		return "", fmt.Errorf("pending run cannot be requeued from %q", pending.State)
	}
	replacement, err := newCommandID()
	if err != nil {
		return "", err
	}
	pending.State = PendingRunQueued
	pending.Command.CommandID = replacement
	pending.CancelCommandID = ""
	return replacement, nil
}

func (pending PendingRun) acknowledgeable() error {
	if pending.State != PendingRunDispatching && pending.State != PendingRunCanceling {
		return fmt.Errorf("pending run cannot be acknowledged from %q", pending.State)
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
	mu             sync.Mutex
	directory      string
	historyLimit   int
	stashLimit     int
	workspaceLimit int
	now            func() time.Time
	random         io.Reader
	history        []agent.Message
	drafts         map[string]agent.Message
	stashes        []Stash
	workspaces     []Workspace
	pendingRuns    map[string][]PendingRun
	pendingResumes map[string]PendingResume
}

// Open loads a file-backed store. An empty directory creates a memory-only
// store, which keeps embedders and in-memory tests explicit.
func Open(directory string, config Config) (*Store, error) {
	store := &Store{
		directory:      strings.TrimSpace(directory),
		historyLimit:   positiveOr(config.HistoryLimit, defaultHistoryLimit),
		stashLimit:     positiveOr(config.StashLimit, defaultStashLimit),
		workspaceLimit: positiveOr(config.WorkspaceLimit, defaultWorkspaceCap),
		now:            config.Now,
		random:         config.Random,
		drafts:         make(map[string]agent.Message),
		pendingRuns:    make(map[string][]PendingRun),
		pendingResumes: make(map[string]PendingResume),
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
	if err := store.loadOptional("stashes.json", &store.stashes); err != nil {
		return nil, fmt.Errorf("load prompt stashes: %w", err)
	}
	if err := store.loadOptional("workspaces.json", &store.workspaces); err != nil {
		return nil, fmt.Errorf("load recent workspaces: %w", err)
	}
	if err := store.loadSessionStates(); err != nil {
		return nil, fmt.Errorf("load session authoring state: %w", err)
	}
	store.history = tailMessages(store.history, store.historyLimit)
	store.stashes = tailStashes(store.stashes, store.stashLimit)
	store.workspaces = slices.Clone(store.workspaces[:min(len(store.workspaces), store.workspaceLimit)])
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

func (s *Store) MarkPendingRunDispatching(sessionID string, commandID agent.CommandID) error {
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
	if err := next[sessionID][index].beginDispatch(); err != nil {
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
func (s *Store) MarkPendingRunCanceling(sessionID string, commandID agent.CommandID) (agent.CommandID, error) {
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
	cancelCommandID, err := next[sessionID][index].beginCancellation(agent.NewCommandID)
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
	nextHistory := cloneMessages(s.history)
	if len(nextHistory) == 0 || !nextHistory[len(nextHistory)-1].Equal(message) {
		nextHistory = tailMessages(append(nextHistory, message), s.historyLimit)
		if err := s.save("history.json", nextHistory); err != nil {
			return err
		}
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
	s.history = nextHistory
	s.pendingRuns = next
	return nil
}

// History returns detached prompts in oldest-to-newest order.
func (s *Store) History() []agent.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneMessages(s.history)
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
	next := append(cloneMessages(s.history), message)
	next = tailMessages(next, s.historyLimit)
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
	if current, ok := s.drafts[sessionID]; ok && current.Equal(message) {
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
// session: its draft, pending run commands, and pending HITL command. Session
// deletion cannot safely compose the three narrower mutations because each
// rewrites the same durable aggregate and a failure between them would expose a
// partially retired session after restart.
func (s *Store) RetireSessionState(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveSessionStateWithResume(sessionID, agent.Message{}, nil, nil); err != nil {
		return err
	}
	delete(s.drafts, sessionID)
	delete(s.pendingRuns, sessionID)
	delete(s.pendingResumes, sessionID)
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
	}
	return nil
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
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	name := s.sessionStateName(sessionID)
	if messageEmpty(draft) && len(pending) == 0 && resume == nil {
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

func tailMessages(messages []agent.Message, limit int) []agent.Message {
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return cloneMessages(messages)
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

func cloneMessages(messages []agent.Message) []agent.Message {
	out := make([]agent.Message, len(messages))
	for i, message := range messages {
		out[i] = message.Clone()
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
	return left.State == right.State && left.CancelCommandID == right.CancelCommandID &&
		left.Command.Equal(right.Command)
}

func pendingResumeEqual(left, right PendingResume) bool {
	return left.Command.Equal(right.Command) && agent.InteractionsEqual(left.Interactions, right.Interactions)
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

func messageEmpty(message agent.Message) bool {
	return strings.TrimSpace(message.Text) == "" && len(message.Attachments) == 0
}
