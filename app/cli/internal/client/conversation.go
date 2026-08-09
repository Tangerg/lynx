package client

import (
	"errors"
	"fmt"
	"slices"
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

	phase    Phase
	runID    string
	index    map[string]int
	events   map[Cursor]string
	cursor   Cursor
	revision uint64
}

// NewConversation returns an empty aggregate.
func NewConversation() *Conversation {
	return &Conversation{index: make(map[string]int), events: make(map[Cursor]string)}
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
func (c *Conversation) Interaction() Interaction { return cloneInteraction(c.interrupt) }
func (c *Conversation) Outcome() Outcome         { return c.outcome }
func (c *Conversation) Phase() Phase             { return c.phase }
func (c *Conversation) RunID() string            { return c.runID }
func (c *Conversation) Revision() uint64         { return c.revision }
func (c *Conversation) Cursor() Cursor           { return c.cursor }
func (c *Conversation) Busy() bool               { return c.phase != Idle }

// ApplyEnvelope folds one durable event exactly once. Replayed duplicates are
// accepted without changing presentation state; gaps and conflicting cursor
// identities are rejected so callers can resnapshot instead of hiding loss.
func (c *Conversation) ApplyEnvelope(envelope Envelope) (ApplyResult, error) {
	if envelope.Cursor == 0 {
		return ApplyResult{}, errors.New("conversation: event cursor is zero")
	}
	if envelope.ID == "" {
		return ApplyResult{}, errors.New("conversation: event id is empty")
	}
	if envelope.Event == nil {
		return ApplyResult{}, errors.New("conversation: envelope has no event")
	}
	if c.events == nil {
		c.events = make(map[Cursor]string)
	}
	if known, ok := c.events[envelope.Cursor]; ok {
		if known != envelope.ID {
			return ApplyResult{}, fmt.Errorf("%w at cursor %d: have %s, received %s", ErrEventConflict, envelope.Cursor, known, envelope.ID)
		}
		return ApplyResult{}, nil
	}
	want := c.cursor + 1
	if envelope.Cursor != want {
		return ApplyResult{}, fmt.Errorf("%w: expected cursor %d, received %d", ErrEventGap, want, envelope.Cursor)
	}
	if err := c.Apply(envelope.Event); err != nil {
		return ApplyResult{}, err
	}
	c.events[envelope.Cursor] = envelope.ID
	c.cursor = envelope.Cursor
	return ApplyResult{Applied: true}, nil
}

// Restore atomically rebuilds a conversation from an authoritative snapshot.
func (c *Conversation) Restore(events []Envelope) error {
	next := NewConversation()
	for _, envelope := range events {
		if _, err := next.ApplyEnvelope(envelope); err != nil {
			return fmt.Errorf("restore conversation at cursor %d: %w", envelope.Cursor, err)
		}
	}
	*c = *next
	return nil
}

// Apply folds one event and rejects any transition that would hide a broken
// event stream.
func (c *Conversation) Apply(event Event) error {
	if event == nil {
		return errors.New("conversation: nil event")
	}
	if c.index == nil {
		c.index = make(map[string]int)
	}
	changed := true
	switch e := event.(type) {
	case RunStarted:
		if e.RunID == "" {
			return errors.New("conversation: run started without an id")
		}
		if c.phase == Waiting || (c.phase == Running && c.runID != "") {
			return fmt.Errorf("%w: cannot start %s while %s is active", ErrInvalidTransition, e.RunID, c.runID)
		}
		c.runID = e.RunID
		c.phase = Running
		c.plan = nil
		c.usage = Usage{}
		c.outcome = Outcome{}
		c.interrupt = nil
	case RunResumed:
		if c.phase != Waiting || c.runID == "" {
			return fmt.Errorf("%w: cannot resume a run that is not waiting", ErrInvalidTransition)
		}
		if e.InterruptID == "" || interactionID(c.interrupt) != e.InterruptID {
			return fmt.Errorf("%w: interrupt %s is not active", ErrInvalidTransition, e.InterruptID)
		}
		c.phase = Running
		c.interrupt = nil
	case BlockStarted:
		if err := c.put(e.Block, false); err != nil {
			return err
		}
	case BlockDelta:
		at, ok := c.index[e.BlockID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownBlock, e.BlockID)
		}
		c.blocks[at].Text += e.Text
	case BlockCompleted:
		if err := c.put(e.Block, true); err != nil {
			return err
		}
	case PlanChanged:
		c.plan = slices.Clone(e.Items)
	case RunInterrupted:
		if e.Interaction == nil || interactionID(e.Interaction) == "" {
			return errors.New("conversation: run parked without an interrupt id")
		}
		if c.phase != Running || c.runID == "" {
			return fmt.Errorf("%w: cannot park a run that has not started", ErrInvalidTransition)
		}
		c.phase = Waiting
		c.interrupt = cloneInteraction(e.Interaction)
	case RunFinished:
		c.phase = Idle
		c.interrupt = nil
		c.outcome = e.Outcome
		c.usage = e.Usage
	default:
		changed = false
	}
	if changed {
		c.revision++
	}
	return nil
}

// Starting records the request-to-first-event window as busy.
func (c *Conversation) Starting() {
	c.phase = Running
	c.runID = ""
	c.plan = nil
	c.usage = Usage{}
	c.outcome = Outcome{}
	c.interrupt = nil
	c.revision++
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

// Reset releases the conversation while preserving a monotonic revision.
func (c *Conversation) Reset() {
	revision := c.revision + 1
	*c = Conversation{index: make(map[string]int), events: make(map[Cursor]string), revision: revision}
}

// ClearPresentation releases transcript and settled run details while keeping
// replay identity. The next event in the same session must still follow Cursor.
func (c *Conversation) ClearPresentation() {
	c.blocks = nil
	c.plan = nil
	c.usage = Usage{}
	c.outcome = Outcome{}
	c.index = make(map[string]int)
	c.revision++
}

func interactionID(interaction Interaction) string {
	switch item := interaction.(type) {
	case Approval:
		return item.InterruptID
	case Question:
		return item.InterruptID
	default:
		return ""
	}
}

func cloneInteraction(interaction Interaction) Interaction {
	switch item := interaction.(type) {
	case Approval:
		return item
	case Question:
		copy := item
		copy.Fields = make([]QuestionField, len(item.Fields))
		for i, field := range item.Fields {
			copy.Fields[i] = field
			copy.Fields[i].Options = slices.Clone(field.Options)
		}
		return copy
	default:
		return nil
	}
}

func (c *Conversation) put(block Block, completed bool) error {
	if block.ID == "" {
		return errors.New("conversation: transcript block has no id")
	}
	if block.Kind == BlockTool {
		if block.Tool == nil {
			return errors.New("conversation: tool block has no tool projection")
		}
		if err := block.Tool.Validate(); err != nil {
			return fmt.Errorf("conversation: block %s: %w", block.ID, err)
		}
		if !completed && block.Tool.Status != ToolRunning {
			return fmt.Errorf("conversation: block %s started with tool status %q", block.ID, block.Tool.Status)
		}
		if completed && block.Tool.Status == ToolRunning {
			return fmt.Errorf("conversation: block %s completed while tool is still running", block.ID)
		}
	} else if block.Tool != nil {
		return fmt.Errorf("conversation: %s block %s carries a tool projection", block.Kind, block.ID)
	}
	if at, ok := c.index[block.ID]; ok {
		c.blocks[at] = cloneBlock(block)
		return nil
	}
	if !completed {
		c.index[block.ID] = len(c.blocks)
		c.blocks = append(c.blocks, cloneBlock(block))
		return nil
	}
	// A complete non-streamed block legitimately arrives without BlockStarted.
	c.index[block.ID] = len(c.blocks)
	c.blocks = append(c.blocks, cloneBlock(block))
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
