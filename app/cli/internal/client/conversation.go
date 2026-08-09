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
	blocks   []Block
	plan     []PlanItem
	usage    Usage
	approval Approval
	outcome  Outcome

	phase    Phase
	runID    string
	index    map[string]int
	revision uint64
}

// NewConversation returns an empty aggregate.
func NewConversation() *Conversation {
	return &Conversation{index: make(map[string]int)}
}

func (c *Conversation) Blocks() []Block {
	out := make([]Block, len(c.blocks))
	for i, block := range c.blocks {
		out[i] = cloneBlock(block)
	}
	return out
}
func (c *Conversation) Plan() []PlanItem   { return slices.Clone(c.plan) }
func (c *Conversation) Usage() Usage       { return c.usage }
func (c *Conversation) Approval() Approval { return c.approval }
func (c *Conversation) Outcome() Outcome   { return c.outcome }
func (c *Conversation) Phase() Phase       { return c.phase }
func (c *Conversation) RunID() string      { return c.runID }
func (c *Conversation) Revision() uint64   { return c.revision }
func (c *Conversation) Busy() bool         { return c.phase != Idle }

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
		c.approval = Approval{}
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
	case RunParked:
		if e.Approval.InterruptID == "" {
			return errors.New("conversation: run parked without an interrupt id")
		}
		if c.phase != Running || c.runID == "" {
			return fmt.Errorf("%w: cannot park a run that has not started", ErrInvalidTransition)
		}
		c.phase = Waiting
		c.approval = e.Approval
	case RunFinished:
		c.phase = Idle
		c.approval = Approval{}
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
	c.approval = Approval{}
	c.revision++
}

// Resumed records the outbound answer that resumes a parked run.
func (c *Conversation) Resumed() bool {
	if c.phase != Waiting {
		return false
	}
	c.phase = Running
	c.approval = Approval{}
	c.revision++
	return true
}

// Failed settles a run whose stream ended without a RunFinished event.
func (c *Conversation) Failed(err error) {
	if err == nil {
		return
	}
	c.phase = Idle
	c.approval = Approval{}
	c.outcome = Outcome{Status: OutcomeFailed, Error: err.Error()}
	_ = c.put(Block{ID: fmt.Sprintf("failure:%d", c.revision+1), Kind: BlockError, Text: err.Error()}, true)
	c.revision++
}

// Reset releases the conversation while preserving a monotonic revision.
func (c *Conversation) Reset() {
	revision := c.revision + 1
	*c = Conversation{index: make(map[string]int), revision: revision}
}

func (c *Conversation) put(block Block, completed bool) error {
	if block.ID == "" {
		return errors.New("conversation: transcript block has no id")
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
	if block.Tool != nil {
		tool := *block.Tool
		block.Tool = &tool
	}
	return block
}
