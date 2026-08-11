package agent

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type SessionStatus string

const (
	SessionRunning SessionStatus = "running"
	SessionWaiting SessionStatus = "waiting"
	SessionIdle    SessionStatus = "idle"
)

type Session struct {
	ID        string
	Title     string
	Status    SessionStatus
	Model     string
	Workspace string
	CreatedAt time.Time
	UpdatedAt time.Time
	Favorite  bool
	Revision  uint64
}

func (s Session) Validate() error {
	var problems []error
	if strings.TrimSpace(s.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if strings.TrimSpace(s.Workspace) == "" {
		problems = append(problems, errors.New("workspace is empty"))
	}
	if s.Status != SessionRunning && s.Status != SessionWaiting && s.Status != SessionIdle {
		problems = append(problems, fmt.Errorf("status %q is invalid", s.Status))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	return nil
}

type SessionQuery struct {
	Cursor    string
	Limit     int
	Search    string
	Workspace string
}

type SessionPage struct {
	Items      []Session
	NextCursor string
}

func (p SessionPage) Validate() error {
	seen := make(map[string]struct{}, len(p.Items))
	for index, session := range p.Items {
		if err := session.Validate(); err != nil {
			return fmt.Errorf("session page item %d: %w", index+1, err)
		}
		if _, duplicate := seen[session.ID]; duplicate {
			return fmt.Errorf("session page repeats id %q", session.ID)
		}
		seen[session.ID] = struct{}{}
	}
	return nil
}

// SessionSnapshot is the cold-read projection the CLI restores. Transcript,
// Runs, and Plan are durable values, never reconstructed from a historical
// event stream. Runs contains root runs in creation order.
type SessionSnapshot struct {
	Session      Session
	Transcript   []Block
	Runs         []Run
	Plan         []PlanItem
	PlanRevision uint64
	Interactions []Interaction
}

// LatestRun returns the most recently created root run.
func (s SessionSnapshot) LatestRun() (Run, bool) {
	if len(s.Runs) == 0 {
		return Run{}, false
	}
	return s.Runs[len(s.Runs)-1].Clone(), true
}

// ActiveRun returns the sole running or waiting root run, when one exists.
func (s SessionSnapshot) ActiveRun() (Run, bool) {
	for _, run := range slices.Backward(s.Runs) {
		if run.Status != RunStatusFinished {
			return run.Clone(), true
		}
	}
	return Run{}, false
}

func (s SessionSnapshot) RunByID(id string) (Run, bool) {
	for _, run := range s.Runs {
		if run.ID == id {
			return run.Clone(), true
		}
	}
	return Run{}, false
}

func (s SessionSnapshot) Validate() error {
	if err := s.Session.Validate(); err != nil {
		return fmt.Errorf("session snapshot: %w", err)
	}
	blocksByIdentity := make(map[string]Block, len(s.Transcript))
	var runningBlocks []Block
	for i, block := range s.Transcript {
		if err := validateBlock(block, block.Status != BlockStatusRunning); err != nil {
			return fmt.Errorf("session snapshot: transcript block %d: %w", i+1, err)
		}
		identity := block.RunID + "\x00" + block.ID
		if _, duplicate := blocksByIdentity[identity]; duplicate {
			return fmt.Errorf("session snapshot: transcript repeats block %q in run %q", block.ID, block.RunID)
		}
		blocksByIdentity[identity] = block
		if block.Status == BlockStatusRunning {
			if block.Kind != BlockTool {
				return fmt.Errorf("session snapshot: transcript block %d: only a tool call can be durably running", i+1)
			}
			runningBlocks = append(runningBlocks, block)
		}
	}
	runIDs := make(map[string]struct{}, len(s.Runs))
	activeIndex := -1
	for i, run := range s.Runs {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("session snapshot: run %d: %w", i+1, err)
		}
		if run.SessionID != s.Session.ID {
			return fmt.Errorf("session snapshot: run %s belongs to session %s", run.ID, run.SessionID)
		}
		if _, duplicate := runIDs[run.ID]; duplicate {
			return fmt.Errorf("session snapshot: repeats run %q", run.ID)
		}
		runIDs[run.ID] = struct{}{}
		if run.Status != RunStatusFinished {
			if activeIndex >= 0 {
				return errors.New("session snapshot: more than one root run is active")
			}
			activeIndex = i
		}
	}
	for _, block := range s.Transcript {
		if _, exists := runIDs[block.RunID]; !exists {
			return fmt.Errorf("session snapshot: block %s references unknown run %s", block.ID, block.RunID)
		}
	}
	if activeIndex >= 0 && activeIndex != len(s.Runs)-1 {
		return errors.New("session snapshot: active run is not the latest root run")
	}
	if err := validatePlan(s.Plan); err != nil {
		return fmt.Errorf("session snapshot: %w", err)
	}
	if s.PlanRevision == 0 && len(s.Plan) != 0 {
		return errors.New("session snapshot: unwritten plan contains items")
	}
	if activeIndex < 0 {
		if len(runningBlocks) != 0 {
			return errors.New("session snapshot: idle session carries a running transcript block")
		}
		if len(s.Interactions) != 0 {
			return errors.New("session snapshot: idle session carries pending interactions")
		}
		if s.Session.Status != SessionIdle {
			return fmt.Errorf("session snapshot: session status is %s without an active run", s.Session.Status)
		}
		return nil
	}
	active := s.Runs[activeIndex]
	for _, block := range runningBlocks {
		if block.RunID != active.ID {
			return fmt.Errorf("session snapshot: running block %s belongs to inactive run %s", block.ID, block.RunID)
		}
	}
	switch active.Status {
	case RunStatusRunning:
		if s.Session.Status != SessionRunning {
			return fmt.Errorf("session snapshot: running run has session status %s", s.Session.Status)
		}
		if len(s.Interactions) != 0 {
			return errors.New("session snapshot: running run carries pending interactions")
		}
	case RunStatusWaiting:
		if s.Session.Status != SessionWaiting {
			return fmt.Errorf("session snapshot: waiting run has session status %s", s.Session.Status)
		}
		if err := ValidateInteractions(s.Interactions); err != nil {
			return fmt.Errorf("session snapshot: waiting run: %w", err)
		}
		for _, interaction := range s.Interactions {
			itemID := InteractionItemID(interaction)
			block, exists := blocksByIdentity[blockIdentity(active.ID, itemID)]
			if !exists {
				return fmt.Errorf("session snapshot: waiting interrupt references unknown item %s", itemID)
			}
			if err := validateInteractionItem(interaction, block); err != nil {
				return fmt.Errorf("session snapshot: waiting run: %w", err)
			}
		}
	}
	return nil
}

func (c *Conversation) RestoreSnapshot(snapshot SessionSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	next := NewConversation()
	next.blocks = cloneBlocks(snapshot.Transcript)
	next.plan = append([]PlanItem(nil), snapshot.Plan...)
	next.planRevision = snapshot.PlanRevision
	next.rebuildBlockIndex()
	if active, ok := snapshot.ActiveRun(); ok {
		next.runID = active.ID
		next.segmentID = active.ActiveSegmentID
		next.usage = active.Usage.Clone()
		if active.Status == RunStatusWaiting {
			next.phase = ConversationWaiting
			next.interactions = CloneInteractions(snapshot.Interactions)
		} else {
			next.phase = ConversationRunning
			next.coldTail = true
		}
	} else if latest, ok := snapshot.LatestRun(); ok {
		next.runID = latest.ID
		next.usage = latest.Usage.Clone()
		next.outcome = latest.Outcome
	}
	*c = *next
	return nil
}

// RestoreAttachedSnapshot restores a cold projection that was read after a
// cursorless subscription was attached. HeadEventID is the exact journal
// position preceding that stream; retaining it closes a second-disconnect gap
// before the first replayable event arrives.
func (c *Conversation) RestoreAttachedSnapshot(snapshot SessionSnapshot, stream SegmentStream) error {
	if err := stream.ValidateSubscription(); err != nil {
		return fmt.Errorf("restore attached snapshot: %w", err)
	}
	active, ok := snapshot.ActiveRun()
	if !ok || active.Status != RunStatusRunning {
		return errors.New("restore attached snapshot: snapshot has no running run")
	}
	if active.ID != stream.RunID || active.ActiveSegmentID != stream.SegmentID {
		return fmt.Errorf(
			"restore attached snapshot: stream %s/%s does not match run %s/%s",
			stream.RunID, stream.SegmentID, active.ID, active.ActiveSegmentID,
		)
	}
	if err := c.RestoreSnapshot(snapshot); err != nil {
		return err
	}
	c.checkpoint = stream.HeadEventID
	c.reconciling = true
	return nil
}

type CreateSession struct {
	Title     string
	Workspace string
}

type UpdateSession struct {
	SessionID        string
	Title            string
	ExpectedRevision uint64
}

type ForkSession struct {
	SessionID string
	FromRunID string
	Title     string
}

type DeleteSession struct{ SessionID string }

func cloneBlocks(blocks []Block) []Block {
	out := make([]Block, len(blocks))
	for i, block := range blocks {
		out[i] = block.Clone()
	}
	return out
}
