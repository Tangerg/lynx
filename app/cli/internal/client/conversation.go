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
	return NewConversationAt(0)
}

// NewConversationAt returns an empty aggregate whose next event follows after.
// Delivery adapters use it for a run-scoped replay window that begins inside a
// longer session transcript.
func NewConversationAt(after Cursor) *Conversation {
	return &Conversation{index: make(map[string]int), open: make(map[string]bool), sequence: NewEventSequence(after)}
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
	if err := validateConversationEnvelope(envelope); err != nil {
		return ApplyResult{}, err
	}
	if c.sequence == nil {
		c.sequence = NewEventSequence(0)
	}
	return c.sequence.Accept(envelope, func() error {
		if err := c.validateEnvelopeIdentity(envelope); err != nil {
			return err
		}
		if err := c.apply(envelope.Event); err != nil {
			return err
		}
		c.sessionID = envelope.SessionID
		return nil
	})
}

func validateConversationEnvelope(envelope Envelope) error {
	switch {
	case envelope.Event == nil:
		return errors.New("conversation: envelope has no event")
	case strings.TrimSpace(envelope.RunID) == "":
		return errors.New("conversation: envelope has no run id")
	case strings.TrimSpace(envelope.SessionID) == "":
		return errors.New("conversation: envelope has no session id")
	default:
		return nil
	}
}

func (c *Conversation) validateEnvelopeIdentity(envelope Envelope) error {
	if c.sessionID != "" && envelope.SessionID != c.sessionID {
		return fmt.Errorf("conversation: event session %s does not match %s", envelope.SessionID, c.sessionID)
	}
	started, isStart := envelope.Event.(RunStarted)
	if isStart {
		if started.RunID != envelope.RunID || started.SessionID != envelope.SessionID {
			return errors.New("conversation: run-start identity does not match its envelope")
		}
		return nil
	}
	if c.runID == "" || envelope.RunID != c.runID {
		return fmt.Errorf("conversation: event run %s does not match active run %s", envelope.RunID, c.runID)
	}
	return nil
}

// apply folds one event after ApplyEnvelope has checked its replay identity.
func (c *Conversation) apply(event Event) error {
	if err := ValidateEvent(event); err != nil {
		return fmt.Errorf("conversation: %w", err)
	}
	c.ensureStorage()
	handled, err := c.applyRunEvent(event)
	if !handled {
		err = c.applyProgressEvent(event)
	}
	if err != nil {
		return err
	}
	c.revision++
	return nil
}

func (c *Conversation) applyRunEvent(event Event) (bool, error) {
	switch e := event.(type) {
	case RunStarted:
		return true, c.applyStarted(e)
	case RunResumed:
		return true, c.applyResumed(e)
	case RunInterrupted:
		return true, c.applyInterrupted(e)
	case RunFinished:
		return true, c.applyFinished(e)
	default:
		return false, nil
	}
}

func (c *Conversation) applyProgressEvent(event Event) error {
	switch e := event.(type) {
	case BlockStarted:
		return c.applyBlockStarted(e)
	case BlockDelta:
		return c.applyBlockDelta(e)
	case BlockCompleted:
		return c.applyBlockCompleted(e)
	case PlanChanged:
		return c.applyPlanChanged(e)
	default:
		return fmt.Errorf("conversation: event %T is unsupported", event)
	}
}

func (c *Conversation) applyBlockStarted(event BlockStarted) error {
	if err := c.requireRunning("start a block"); err != nil {
		return err
	}
	return c.put(event.Block, false)
}

func (c *Conversation) applyBlockDelta(event BlockDelta) error {
	if err := c.requireRunning("append a block delta"); err != nil {
		return err
	}
	at, ok := c.index[event.BlockID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownBlock, event.BlockID)
	}
	if !c.open[event.BlockID] {
		return fmt.Errorf("%w: block %s is already complete", ErrInvalidTransition, event.BlockID)
	}
	block := &c.blocks[at]
	switch block.Kind {
	case BlockAssistant, BlockReasoning:
		block.Text += event.Text
	case BlockTool:
		block.Tool.Output += event.Text
	default:
		return fmt.Errorf("%w: block %s of kind %s cannot stream", ErrInvalidTransition, event.BlockID, block.Kind)
	}
	return nil
}

func (c *Conversation) applyBlockCompleted(event BlockCompleted) error {
	if err := c.requireRunning("complete a block"); err != nil {
		return err
	}
	return c.put(event.Block, true)
}

func (c *Conversation) applyPlanChanged(event PlanChanged) error {
	if err := c.requireRunning("change the plan"); err != nil {
		return err
	}
	c.plan = slices.Clone(event.Items)
	return nil
}

func (c *Conversation) applyStarted(event RunStarted) error {
	if c.phase == Waiting || (c.phase == Running && c.runID != "") {
		return fmt.Errorf("%w: cannot start %s while %s is active", ErrInvalidTransition, event.RunID, c.runID)
	}
	c.runID = event.RunID
	c.phase = Running
	c.plan = nil
	c.usage = Usage{}
	c.outcome = Outcome{}
	c.interrupt = nil
	c.index = make(map[string]int)
	c.open = make(map[string]bool)
	return nil
}

func (c *Conversation) applyResumed(event RunResumed) error {
	if c.phase != Waiting || c.runID == "" {
		return fmt.Errorf("%w: cannot resume a run that is not waiting", ErrInvalidTransition)
	}
	if InteractionID(c.interrupt) != event.InterruptID {
		return fmt.Errorf("%w: interrupt %s is not active", ErrInvalidTransition, event.InterruptID)
	}
	c.phase = Running
	c.interrupt = nil
	return nil
}

func (c *Conversation) applyInterrupted(event RunInterrupted) error {
	if c.phase != Running || c.runID == "" {
		return fmt.Errorf("%w: cannot park a run that has not started", ErrInvalidTransition)
	}
	c.phase = Waiting
	c.interrupt = CloneInteraction(event.Interaction)
	return nil
}

func (c *Conversation) applyFinished(event RunFinished) error {
	if c.phase == Idle || c.runID == "" {
		return fmt.Errorf("%w: cannot finish a run that has not started", ErrInvalidTransition)
	}
	if c.phase == Waiting && event.Outcome.Status != OutcomeCanceled {
		return fmt.Errorf("%w: a waiting run can only finish by cancellation", ErrInvalidTransition)
	}
	if event.Outcome.Status == OutcomeCompleted && c.hasOpenBlocks() {
		return fmt.Errorf("%w: a completed run still has open blocks", ErrInvalidTransition)
	}
	toolStatus := ToolError
	if event.Outcome.Status == OutcomeCanceled {
		toolStatus = ToolCanceled
	}
	c.settleOpenBlocks(toolStatus)
	c.phase = Idle
	c.interrupt = nil
	c.outcome = event.Outcome
	c.usage = event.Usage
	return nil
}

func (c *Conversation) ensureStorage() {
	if c.index == nil {
		c.index = make(map[string]int)
	}
	if c.open == nil {
		c.open = make(map[string]bool)
	}
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
	c.settleOpenBlocks(ToolError)
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
	c.ensureStorage()
	if at, ok := c.index[block.ID]; ok {
		if !completed {
			return fmt.Errorf("%w: block %s started twice", ErrInvalidTransition, block.ID)
		}
		if !c.open[block.ID] {
			return fmt.Errorf("%w: block %s completed twice", ErrInvalidTransition, block.ID)
		}
		if err := validateBlockCompletion(c.blocks[at], block); err != nil {
			return err
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

func validateBlockCompletion(started, completed Block) error {
	if started.Kind != completed.Kind {
		return fmt.Errorf("%w: block %s changed kind from %s to %s", ErrInvalidTransition, completed.ID, started.Kind, completed.Kind)
	}
	if started.Kind == BlockTool && started.Tool.Kind != completed.Tool.Kind {
		return fmt.Errorf("%w: tool block %s changed kind from %s to %s", ErrInvalidTransition, completed.ID, started.Tool.Kind, completed.Tool.Kind)
	}
	return nil
}

func (c *Conversation) hasOpenBlocks() bool {
	for _, open := range c.open {
		if open {
			return true
		}
	}
	return false
}

func (c *Conversation) settleOpenBlocks(toolStatus ToolStatus) {
	for id, open := range c.open {
		if !open {
			continue
		}
		block := &c.blocks[c.index[id]]
		if block.Kind == BlockTool && block.Tool != nil {
			block.Tool.Status = toolStatus
		}
		c.open[id] = false
	}
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
