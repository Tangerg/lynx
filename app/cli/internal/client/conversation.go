package client

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	// ErrUnknownBlock means a delta referred to a block the conversation has not
	// observed. It is a protocol divergence, not an invitation to invent state.
	ErrUnknownBlock = errors.New("unknown transcript block")
	// ErrInvalidTransition means otherwise valid data arrived in an impossible
	// conversation phase.
	ErrInvalidTransition = errors.New("invalid conversation transition")
)

// ApplyResult distinguishes a replayed duplicate from newly folded state.
type ApplyResult struct{ Applied bool }

// Phase is what a conversation is doing.
type Phase uint8

const (
	Idle Phase = iota
	Running
	Waiting
)

// Conversation is the aggregate produced by folding a run's ordered events.
// The aggregate is presentation-neutral: terminal and headless adapters read
// the same truth instead of independently reconstructing run state.
type Conversation struct {
	blocks    []Block
	plan      []PlanItem
	usage     Usage
	interrupt Interaction
	outcome   Outcome

	phase     Phase
	runID     string
	sessionID string
	index     map[string]int
	open      map[string]bool
	sequence  *EventSequence
	revision  uint64
}

// NewConversation returns an empty aggregate.
func NewConversation() *Conversation {
	return &Conversation{index: make(map[string]int), open: make(map[string]bool), sequence: NewEventSequence(0)}
}

func (c *Conversation) Blocks() []Block {
	out := make([]Block, len(c.blocks))
	for i, block := range c.blocks {
		out[i] = cloneBlock(block)
	}
	return out
}
func (c *Conversation) Plan() []PlanItem         { return slices.Clone(c.plan) }
func (c *Conversation) Usage() Usage             { return c.usage }
func (c *Conversation) Interaction() Interaction { return CloneInteraction(c.interrupt) }
func (c *Conversation) Outcome() Outcome         { return c.outcome }
func (c *Conversation) Phase() Phase             { return c.phase }
func (c *Conversation) RunID() string            { return c.runID }
func (c *Conversation) Cursor() Cursor {
	if c.sequence == nil {
		return 0
	}
	return c.sequence.Cursor()
}
func (c *Conversation) Busy() bool { return c.phase != Idle }

// ApplyEnvelope folds one durable event exactly once. Replayed duplicates are
// accepted without changing presentation state; gaps and conflicting cursor
// identities are rejected so callers can resnapshot instead of hiding loss.
func (c *Conversation) ApplyEnvelope(envelope Envelope) (ApplyResult, error) {
	if envelope.Event == nil {
		return ApplyResult{}, errors.New("conversation: envelope has no event")
	}
	if strings.TrimSpace(envelope.RunID) == "" {
		return ApplyResult{}, errors.New("conversation: envelope has no run id")
	}
	if strings.TrimSpace(envelope.SessionID) == "" {
		return ApplyResult{}, errors.New("conversation: envelope has no session id")
	}
	if c.sequence == nil {
		c.sequence = NewEventSequence(0)
	}
	return c.sequence.Accept(envelope, func() error {
		if c.sessionID != "" && envelope.SessionID != c.sessionID {
			return fmt.Errorf("conversation: event session %s does not match %s", envelope.SessionID, c.sessionID)
		}
		if started, ok := envelope.Event.(RunStarted); ok {
			if started.RunID != envelope.RunID || started.SessionID != envelope.SessionID {
				return errors.New("conversation: run-start identity does not match its envelope")
			}
		} else if c.runID == "" || envelope.RunID != c.runID {
			return fmt.Errorf("conversation: event run %s does not match active run %s", envelope.RunID, c.runID)
		}
		if err := c.apply(envelope.Event); err != nil {
			return err
		}
		c.sessionID = envelope.SessionID
		return nil
	})
}

// apply folds one event after ApplyEnvelope has checked its replay identity.
func (c *Conversation) apply(event Event) error {
	if err := validateEvent(event); err != nil {
		return fmt.Errorf("conversation: %w", err)
	}
	if c.index == nil {
		c.index = make(map[string]int)
	}
	if c.open == nil {
		c.open = make(map[string]bool)
	}
	changed := true
	switch e := event.(type) {
	case RunStarted:
		if c.phase == Waiting || (c.phase == Running && c.runID != "") {
			return fmt.Errorf("%w: cannot start %s while %s is active", ErrInvalidTransition, e.RunID, c.runID)
		}
		c.runID = e.RunID
		c.phase = Running
		c.plan = nil
		c.usage = Usage{}
		c.outcome = Outcome{}
		c.interrupt = nil
		c.index = make(map[string]int)
		c.open = make(map[string]bool)
	case RunResumed:
		if c.phase != Waiting || c.runID == "" {
			return fmt.Errorf("%w: cannot resume a run that is not waiting", ErrInvalidTransition)
		}
		if InteractionID(c.interrupt) != e.InterruptID {
			return fmt.Errorf("%w: interrupt %s is not active", ErrInvalidTransition, e.InterruptID)
		}
		c.phase = Running
		c.interrupt = nil
	case BlockStarted:
		if err := c.requireRunning("start a block"); err != nil {
			return err
		}
		if err := c.put(e.Block, false); err != nil {
			return err
		}
	case BlockDelta:
		if err := c.requireRunning("append a block delta"); err != nil {
			return err
		}
		at, ok := c.index[e.BlockID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownBlock, e.BlockID)
		}
		if !c.open[e.BlockID] {
			return fmt.Errorf("%w: block %s is already complete", ErrInvalidTransition, e.BlockID)
		}
		c.blocks[at].Text += e.Text
	case BlockCompleted:
		if err := c.requireRunning("complete a block"); err != nil {
			return err
		}
		if err := c.put(e.Block, true); err != nil {
			return err
		}
	case PlanChanged:
		if err := c.requireRunning("change the plan"); err != nil {
			return err
		}
		c.plan = slices.Clone(e.Items)
	case RunInterrupted:
		if c.phase != Running || c.runID == "" {
			return fmt.Errorf("%w: cannot park a run that has not started", ErrInvalidTransition)
		}
		c.phase = Waiting
		c.interrupt = CloneInteraction(e.Interaction)
	case RunFinished:
		if c.phase == Idle || c.runID == "" {
			return fmt.Errorf("%w: cannot finish a run that has not started", ErrInvalidTransition)
		}
		if c.phase == Waiting && e.Outcome.Status != OutcomeCanceled {
			return fmt.Errorf("%w: a waiting run can only finish by cancellation", ErrInvalidTransition)
		}
		c.phase = Idle
		c.interrupt = nil
		c.outcome = e.Outcome
		c.usage = e.Usage
	default:
		return fmt.Errorf("conversation: event %T is unsupported", event)
	}
	if changed {
		c.revision++
	}
	return nil
}

// Starting records the request-to-first-event window as busy.
func (c *Conversation) Starting() error {
	if c.Busy() {
		return fmt.Errorf("%w: conversation is already busy", ErrInvalidTransition)
	}
	c.phase = Running
	c.runID = ""
	c.plan = nil
	c.usage = Usage{}
	c.outcome = Outcome{}
	c.interrupt = nil
	c.revision++
	return nil
}

// CancelStarting settles the request-to-first-event window without inventing a
// durable runtime event.
func (c *Conversation) CancelStarting() error {
	if c.phase != Running || c.runID != "" {
		return fmt.Errorf("%w: conversation is not starting", ErrInvalidTransition)
	}
	c.phase = Idle
	c.outcome = Outcome{Status: OutcomeCanceled}
	c.revision++
	return nil
}

// Failed settles a run whose stream ended without a RunFinished event.
func (c *Conversation) Failed(err error) {
	if err == nil {
		return
	}
	c.phase = Idle
	c.interrupt = nil
	c.outcome = Outcome{Status: OutcomeFailed, Error: err.Error()}
	_ = c.put(Block{ID: fmt.Sprintf("failure:%d", c.revision+1), Kind: BlockError, Text: err.Error()}, true)
	c.revision++
}

// ClearPresentation releases transcript and settled run details while keeping
// replay identity. The next event in the same session must still follow Cursor.
func (c *Conversation) ClearPresentation() {
	c.blocks = nil
	c.plan = nil
	c.usage = Usage{}
	c.outcome = Outcome{}
	c.index = make(map[string]int)
	c.open = make(map[string]bool)
	c.revision++
}

func (c *Conversation) put(block Block, completed bool) error {
	if c.index == nil {
		c.index = make(map[string]int)
	}
	if c.open == nil {
		c.open = make(map[string]bool)
	}
	if at, ok := c.index[block.ID]; ok {
		if !completed {
			return fmt.Errorf("%w: block %s started twice", ErrInvalidTransition, block.ID)
		}
		c.blocks[at] = cloneBlock(block)
		c.open[block.ID] = false
		return nil
	}
	if !completed {
		c.index[block.ID] = len(c.blocks)
		c.blocks = append(c.blocks, cloneBlock(block))
		c.open[block.ID] = true
		return nil
	}
	// A complete non-streamed block legitimately arrives without BlockStarted.
	c.index[block.ID] = len(c.blocks)
	c.blocks = append(c.blocks, cloneBlock(block))
	c.open[block.ID] = false
	return nil
}

func (c *Conversation) requireRunning(action string) error {
	if c.phase != Running || c.runID == "" {
		return fmt.Errorf("%w: cannot %s without an active run", ErrInvalidTransition, action)
	}
	return nil
}

func cloneBlock(block Block) Block {
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
