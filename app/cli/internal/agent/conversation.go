package agent

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

var (
	ErrUnknownBlock      = errors.New("unknown transcript block")
	ErrInvalidTransition = errors.New("invalid conversation transition")
)

type EventAcceptance struct{ Applied bool }

type ConversationPhase uint8

const (
	ConversationIdle ConversationPhase = iota
	ConversationRunning
	ConversationWaiting
)

// Conversation is the terminal-facing aggregate. Durable history is restored
// from Items; live state is folded from one exact segment stream at a time.
type Conversation struct {
	blocks       []Block
	plan         []PlanItem
	planRevision uint64
	usage        Usage
	interactions []Interaction
	outcome      Outcome

	phase       ConversationPhase
	runID       string
	segmentID   string
	checkpoint  string
	seen        map[string]RunEvent
	index       map[string]int
	open        map[string]bool
	revision    uint64
	reconciling bool
	coldTail    bool
}

func NewConversation() *Conversation {
	return &Conversation{
		seen:  make(map[string]RunEvent),
		index: make(map[string]int),
		open:  make(map[string]bool),
	}
}

func (c *Conversation) Blocks() []Block             { return cloneBlocks(c.blocks) }
func (c *Conversation) Plan() []PlanItem            { return slices.Clone(c.plan) }
func (c *Conversation) PlanRevision() uint64        { return c.planRevision }
func (c *Conversation) Usage() Usage                { return c.usage.Clone() }
func (c *Conversation) Interactions() []Interaction { return CloneInteractions(c.interactions) }
func (c *Conversation) Outcome() Outcome            { return c.outcome }
func (c *Conversation) Phase() ConversationPhase    { return c.phase }
func (c *Conversation) RunID() string               { return c.runID }
func (c *Conversation) SegmentID() string           { return c.segmentID }
func (c *Conversation) Checkpoint() string          { return c.checkpoint }
func (c *Conversation) Busy() bool                  { return c.phase != ConversationIdle }

// ApplyRunEvent validates and folds one event exactly once. It never assigns
// ordering meaning to EventID; the stream order is the order of delivery.
func (c *Conversation) ApplyRunEvent(envelope RunEvent) (EventAcceptance, error) {
	if err := validateConversationEvent(envelope); err != nil {
		return EventAcceptance{}, err
	}
	newSegment := c.segmentID != envelope.SegmentID
	if newSegment {
		if _, ok := envelope.Event.(SegmentStarted); !ok {
			return EventAcceptance{}, fmt.Errorf("conversation: event segment %s does not match active segment %s", envelope.SegmentID, c.segmentID)
		}
	} else if known, duplicate := c.seen[envelope.EventID]; duplicate {
		if !reflect.DeepEqual(known, envelope) {
			return EventAcceptance{}, fmt.Errorf("%w: event %s changed on replay", ErrEventConflict, envelope.EventID)
		}
		return EventAcceptance{}, nil
	}
	if err := c.validateEventIdentity(envelope); err != nil {
		return EventAcceptance{}, err
	}
	if err := ValidateEvent(envelope.Event); err != nil {
		return EventAcceptance{}, fmt.Errorf("conversation: %w", err)
	}
	ignored, err := c.ignoreRecoveredOverlap(envelope.Event)
	if err != nil {
		return EventAcceptance{}, err
	}
	if !ignored {
		err = c.apply(envelope.Event)
		if err == nil {
			c.reconciling = false
		}
	}
	if err != nil {
		return EventAcceptance{}, err
	}
	if newSegment {
		c.seen = make(map[string]RunEvent)
		c.checkpoint = ""
	}
	c.segmentID = envelope.SegmentID
	c.seen[envelope.EventID] = envelope.Clone()
	if ReplayableEvent(envelope.Event) {
		c.checkpoint = envelope.EventID
	}
	return EventAcceptance{Applied: !ignored}, nil
}

func (c *Conversation) ignoreRecoveredOverlap(event Event) (bool, error) {
	if delta, ok := event.(BlockDelta); ok {
		key := blockIdentity(c.runID, delta.BlockID)
		if _, exists := c.index[key]; !exists && c.coldTail {
			// Agent-message and reasoning starts are non-durable previews. A
			// head attachment can therefore observe their later deltas without
			// either a replayable start or a cold Item. Their completed Item is
			// authoritative and will restore the missing presentation block.
			return true, nil
		}
	}
	if !c.reconciling {
		return false, nil
	}
	switch item := event.(type) {
	case BlockStarted:
		at, exists := c.index[blockIdentity(item.Block.RunID, item.Block.ID)]
		if !exists {
			return false, nil
		}
		if err := validateBlockIdentity(c.blocks[at], item.Block); err != nil {
			return false, fmt.Errorf("%w: replayed start conflicts with the cold snapshot: %v", ErrEventConflict, err)
		}
		return true, nil
	case BlockDelta:
		key := blockIdentity(c.runID, item.BlockID)
		_, exists := c.index[key]
		return !exists || !c.open[key], nil
	case BlockCompleted:
		key := blockIdentity(item.Block.RunID, item.Block.ID)
		at, exists := c.index[key]
		if !exists || c.open[key] {
			return false, nil
		}
		if !reflect.DeepEqual(c.blocks[at], item.Block) {
			return false, fmt.Errorf("%w: completed block %s differs from the cold snapshot", ErrEventConflict, item.Block.ID)
		}
		return true, nil
	case PlanChanged:
		if item.Revision > c.planRevision {
			return false, nil
		}
		if item.Revision == c.planRevision && !slices.Equal(c.plan, item.Items) {
			return false, fmt.Errorf("%w: plan revision %d differs from the cold snapshot", ErrEventConflict, item.Revision)
		}
		return true, nil
	default:
		return false, nil
	}
}

func validateConversationEvent(envelope RunEvent) error {
	switch {
	case strings.TrimSpace(envelope.EventID) == "":
		return errors.New("conversation: event id is empty")
	case strings.TrimSpace(envelope.RunID) == "":
		return errors.New("conversation: run id is empty")
	case strings.TrimSpace(envelope.SegmentID) == "":
		return errors.New("conversation: segment id is empty")
	case envelope.Event == nil:
		return errors.New("conversation: event payload is nil")
	default:
		return nil
	}
}

func (c *Conversation) validateEventIdentity(envelope RunEvent) error {
	if started, ok := envelope.Event.(SegmentStarted); ok {
		if started.Run.ID != envelope.RunID || started.Run.ActiveSegmentID != envelope.SegmentID {
			return errors.New("conversation: segment-start identity does not match its event")
		}
		if c.phase == ConversationWaiting && c.runID != envelope.RunID {
			return fmt.Errorf("conversation: resumed run %s does not match waiting run %s", envelope.RunID, c.runID)
		}
		return nil
	}
	if c.runID == "" || envelope.RunID != c.runID {
		return fmt.Errorf("conversation: event run %s does not match active run %s", envelope.RunID, c.runID)
	}
	if c.segmentID == "" || envelope.SegmentID != c.segmentID {
		return fmt.Errorf("conversation: event segment %s does not match active segment %s", envelope.SegmentID, c.segmentID)
	}
	switch item := envelope.Event.(type) {
	case BlockStarted:
		if item.Block.RunID != envelope.RunID {
			return fmt.Errorf("conversation: block %s belongs to run %s, not %s", item.Block.ID, item.Block.RunID, envelope.RunID)
		}
	case BlockCompleted:
		if item.Block.RunID != envelope.RunID {
			return fmt.Errorf("conversation: block %s belongs to run %s, not %s", item.Block.ID, item.Block.RunID, envelope.RunID)
		}
	}
	return nil
}

func (c *Conversation) apply(event Event) error {
	c.ensureStorage()
	var err error
	switch item := event.(type) {
	case SegmentStarted:
		err = c.applySegmentStarted(item)
	case BlockStarted:
		err = c.applyBlockStarted(item)
	case BlockDelta:
		err = c.applyBlockDelta(item)
	case BlockCompleted:
		err = c.applyBlockCompleted(item)
	case PlanChanged:
		err = c.applyPlanChanged(item)
	case RunInterrupted:
		err = c.applyInterrupted(item)
	case RunFinished:
		err = c.applyFinished(item)
	default:
		err = fmt.Errorf("conversation: event %T is unsupported", event)
	}
	if err != nil {
		return err
	}
	c.revision++
	return nil
}

func (c *Conversation) applySegmentStarted(event SegmentStarted) error {
	c.reconciling = false
	c.coldTail = false
	previousUsage := c.usage
	switch c.phase {
	case ConversationIdle:
		c.outcome = Outcome{}
		previousUsage = Usage{}
	case ConversationWaiting:
		if c.runID != event.Run.ID {
			return fmt.Errorf("%w: cannot resume %s while %s is waiting", ErrInvalidTransition, event.Run.ID, c.runID)
		}
	case ConversationRunning:
		if c.runID != "" {
			return fmt.Errorf("%w: cannot start segment while %s is running", ErrInvalidTransition, c.runID)
		}
	}
	if err := validateUsageProgress(previousUsage, event.Run.Usage); err != nil {
		return fmt.Errorf("%w: segment started with %v", ErrInvalidTransition, err)
	}
	c.runID = event.Run.ID
	c.phase = ConversationRunning
	c.interactions = nil
	c.usage = event.Run.Usage.Clone()
	return nil
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
	key := blockIdentity(c.runID, event.BlockID)
	at, ok := c.index[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownBlock, event.BlockID)
	}
	if !c.open[key] {
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
	if event.Revision <= c.planRevision {
		return fmt.Errorf("%w: plan revision %d does not advance %d", ErrInvalidTransition, event.Revision, c.planRevision)
	}
	c.plan = slices.Clone(event.Items)
	c.planRevision = event.Revision
	return nil
}

func (c *Conversation) applyInterrupted(event RunInterrupted) error {
	if err := c.requireRunning("interrupt a run"); err != nil {
		return err
	}
	for _, interaction := range event.Interactions {
		itemID := InteractionItemID(interaction)
		at, exists := c.index[blockIdentity(c.runID, itemID)]
		if !exists {
			return fmt.Errorf("%w: interrupt references unknown item %s", ErrInvalidTransition, itemID)
		}
		if err := validateInteractionItem(interaction, c.blocks[at]); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidTransition, err)
		}
	}
	if err := validateUsageProgress(c.usage, event.Usage); err != nil {
		return fmt.Errorf("%w: run interrupted with %v", ErrInvalidTransition, err)
	}
	c.phase = ConversationWaiting
	c.reconciling = false
	c.coldTail = false
	c.interactions = CloneInteractions(event.Interactions)
	c.usage = event.Usage.Clone()
	return nil
}

func (c *Conversation) applyFinished(event RunFinished) error {
	if c.phase == ConversationIdle || c.runID == "" {
		return fmt.Errorf("%w: cannot finish a run that has not started", ErrInvalidTransition)
	}
	if c.phase == ConversationWaiting && event.Outcome.Status != OutcomeCanceled {
		return fmt.Errorf("%w: a waiting run can only finish by cancellation", ErrInvalidTransition)
	}
	if event.Outcome.Status == OutcomeCompleted && c.hasOpenBlocks() {
		return fmt.Errorf("%w: a completed run still has open blocks", ErrInvalidTransition)
	}
	if err := validateUsageProgress(c.usage, event.Usage); err != nil {
		return fmt.Errorf("%w: run finished with %v", ErrInvalidTransition, err)
	}
	toolStatus := ToolError
	if event.Outcome.Status == OutcomeCanceled {
		toolStatus = ToolCanceled
	}
	c.settleOpenBlocks(toolStatus)
	c.phase = ConversationIdle
	c.reconciling = false
	c.coldTail = false
	c.interactions = nil
	c.outcome = event.Outcome
	c.usage = event.Usage.Clone()
	return nil
}

func (c *Conversation) Starting() error {
	if c.Busy() {
		return fmt.Errorf("%w: conversation is already busy", ErrInvalidTransition)
	}
	c.phase = ConversationRunning
	c.runID = ""
	c.segmentID = ""
	c.checkpoint = ""
	c.seen = make(map[string]RunEvent)
	c.usage = Usage{}
	c.outcome = Outcome{}
	c.interactions = nil
	c.reconciling = false
	c.coldTail = false
	c.revision++
	return nil
}

func (c *Conversation) CancelStarting() error {
	if c.phase != ConversationRunning || c.runID != "" {
		return fmt.Errorf("%w: conversation is not starting", ErrInvalidTransition)
	}
	c.phase = ConversationIdle
	c.reconciling = false
	c.coldTail = false
	c.outcome = Outcome{Status: OutcomeCanceled}
	c.revision++
	return nil
}

// SettleRun applies the authoritative result of an out-of-band control such as
// runs.cancel, whose response is durable even when no segment stream is open.
func (c *Conversation) SettleRun(run Run) error {
	if err := run.Validate(); err != nil {
		return err
	}
	if run.Status != RunStatusFinished {
		return errors.New("cannot settle conversation from an unfinished run")
	}
	if c.runID != "" && c.runID != run.ID {
		return fmt.Errorf("%w: settled run %s does not match %s", ErrInvalidTransition, run.ID, c.runID)
	}
	if c.runID == run.ID {
		if err := validateUsageProgress(c.usage, run.Usage); err != nil {
			return fmt.Errorf("%w: settled run has %v", ErrInvalidTransition, err)
		}
	}
	toolStatus := ToolError
	if run.Outcome.Status == OutcomeCanceled {
		toolStatus = ToolCanceled
	}
	c.settleOpenBlocks(toolStatus)
	c.runID = run.ID
	c.segmentID = ""
	c.phase = ConversationIdle
	c.interactions = nil
	c.reconciling = false
	c.coldTail = false
	c.outcome = run.Outcome
	c.usage = run.Usage.Clone()
	c.revision++
	return nil
}

func (c *Conversation) Failed(err error) {
	if err == nil {
		return
	}
	c.phase = ConversationIdle
	c.interactions = nil
	c.reconciling = false
	c.coldTail = false
	c.outcome = Outcome{Status: OutcomeFailed, Error: err.Error()}
	c.settleOpenBlocks(ToolError)
	_ = c.put(Block{ID: fmt.Sprintf("failure:%d", c.revision+1), RunID: c.runID, Status: BlockStatusIncomplete, Kind: BlockError, Text: err.Error()}, true)
	c.revision++
}

func (c *Conversation) ClearPresentation() {
	c.blocks = nil
	c.plan = nil
	c.planRevision = 0
	c.usage = Usage{}
	c.outcome = Outcome{}
	c.index = make(map[string]int)
	c.open = make(map[string]bool)
	c.revision++
}

func (c *Conversation) put(block Block, completed bool) error {
	c.ensureStorage()
	key := blockIdentity(block.RunID, block.ID)
	if at, ok := c.index[key]; ok {
		if !completed {
			return fmt.Errorf("%w: block %s started twice", ErrInvalidTransition, block.ID)
		}
		if !c.open[key] {
			return fmt.Errorf("%w: block %s completed twice", ErrInvalidTransition, block.ID)
		}
		if err := validateBlockIdentity(c.blocks[at], block); err != nil {
			return err
		}
		c.blocks[at] = block.Clone()
		c.open[key] = block.Status == BlockStatusRunning
		return nil
	}
	c.index[key] = len(c.blocks)
	c.blocks = append(c.blocks, block.Clone())
	c.open[key] = !completed
	return nil
}

func validateBlockIdentity(started, completed Block) error {
	if started.Kind != completed.Kind {
		return fmt.Errorf("%w: block %s changed kind from %s to %s", ErrInvalidTransition, completed.ID, started.Kind, completed.Kind)
	}
	if started.Kind != BlockTool {
		return nil
	}
	if started.Tool.Kind != completed.Tool.Kind {
		return fmt.Errorf("%w: tool block %s changed kind from %s to %s", ErrInvalidTransition, completed.ID, started.Tool.Kind, completed.Tool.Kind)
	}
	if started.Tool.Name != completed.Tool.Name {
		return fmt.Errorf("%w: tool block %s changed name from %q to %q", ErrInvalidTransition, completed.ID, started.Tool.Name, completed.Tool.Name)
	}
	return nil
}

func (c *Conversation) ensureStorage() {
	if c.seen == nil {
		c.seen = make(map[string]RunEvent)
	}
	if c.index == nil {
		c.index = make(map[string]int)
	}
	if c.open == nil {
		c.open = make(map[string]bool)
	}
}

func (c *Conversation) rebuildBlockIndex() {
	c.index = make(map[string]int, len(c.blocks))
	c.open = make(map[string]bool, len(c.blocks))
	for i, block := range c.blocks {
		key := blockIdentity(block.RunID, block.ID)
		c.index[key] = i
		c.open[key] = block.Status == BlockStatusRunning
	}
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
	for key, open := range c.open {
		if !open {
			continue
		}
		block := &c.blocks[c.index[key]]
		if block.Kind == BlockTool && block.Tool != nil {
			block.Tool.Status = toolStatus
		}
		block.Status = BlockStatusIncomplete
		c.open[key] = false
	}
}

func blockIdentity(runID, blockID string) string { return runID + "\x00" + blockID }

func (c *Conversation) requireRunning(action string) error {
	if c.phase != ConversationRunning || c.runID == "" {
		return fmt.Errorf("%w: cannot %s without an active run", ErrInvalidTransition, action)
	}
	return nil
}
