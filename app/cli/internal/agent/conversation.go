package agent

import (
	"errors"
	"fmt"
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
	runs        map[string]Run
	index       map[string]int
	open        map[string]bool
	textStreams map[string]StreamedText
	revision    uint64
	reconciling bool
	coldTail    bool
}

func NewConversation() *Conversation {
	return &Conversation{
		seen:        make(map[string]RunEvent),
		runs:        make(map[string]Run),
		index:       make(map[string]int),
		open:        make(map[string]bool),
		textStreams: make(map[string]StreamedText),
	}
}

func (c *Conversation) Blocks() []Block             { return cloneBlocks(c.blocks) }
func (c *Conversation) Plan() []PlanItem            { return slices.Clone(c.plan) }
func (c *Conversation) PlanRevision() uint64        { return c.planRevision }
func (c *Conversation) Usage() Usage                { return c.usage.Clone() }
func (c *Conversation) Interactions() []Interaction { return CloneInteractions(c.interactions) }
func (c *Conversation) Outcome() Outcome            { return c.outcome.Clone() }
func (c *Conversation) Phase() ConversationPhase    { return c.phase }
func (c *Conversation) RunID() string               { return c.runID }
func (c *Conversation) SegmentID() string           { return c.segmentID }
func (c *Conversation) Checkpoint() string          { return c.checkpoint }
func (c *Conversation) Busy() bool                  { return c.phase != ConversationIdle }

// MatchesSnapshot reports whether a cold projection carries the same
// conversation state currently folded by this aggregate. Session metadata and
// historical run catalogs are deliberately outside this comparison.
func (c *Conversation) MatchesSnapshot(snapshot SessionSnapshot) bool {
	expected := NewConversation()
	if err := expected.RestoreSnapshot(snapshot); err != nil {
		return false
	}
	if len(c.blocks) != len(expected.blocks) {
		return false
	}
	for index, block := range c.blocks {
		if !block.Equal(expected.blocks[index]) {
			return false
		}
	}
	return slices.Equal(c.plan, expected.plan) && c.planRevision == expected.planRevision &&
		c.usage.Equal(expected.usage) && equalInteractions(c.interactions, expected.interactions) &&
		c.outcome.Equal(expected.outcome) && c.phase == expected.phase && c.runID == expected.runID &&
		c.segmentID == expected.segmentID && equalRunMaps(c.runs, expected.runs)
}

func equalRunMaps(left, right map[string]Run) bool {
	if len(left) != len(right) {
		return false
	}
	for id, run := range left {
		other, exists := right[id]
		if !exists || !run.Equal(other) {
			return false
		}
	}
	return true
}

// ApplyRunEvent validates and folds one event exactly once. It never assigns
// ordering meaning to EventID; the stream order is the order of delivery.
func (c *Conversation) ApplyRunEvent(envelope RunEvent) (EventAcceptance, error) {
	if err := validateConversationEvent(envelope); err != nil {
		return EventAcceptance{}, err
	}
	streamSegmentID := envelope.StreamSegment()
	newStreamSegment := c.segmentID != streamSegmentID
	if newStreamSegment {
		if _, ok := envelope.Event.(SegmentStarted); !ok {
			return EventAcceptance{}, fmt.Errorf("conversation: event stream segment %s does not match active stream segment %s", streamSegmentID, c.segmentID)
		}
	} else if known, duplicate := c.seen[envelope.EventID]; duplicate {
		if !known.Equal(envelope) {
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
	ignored, err := c.ignoreRecoveredOverlap(envelope)
	if err != nil {
		return EventAcceptance{}, err
	}
	if !ignored {
		err = c.apply(envelope)
		if err == nil {
			c.reconciling = false
		}
	}
	if err != nil {
		return EventAcceptance{}, err
	}
	if newStreamSegment {
		c.seen = make(map[string]RunEvent)
		c.checkpoint = ""
	}
	c.segmentID = streamSegmentID
	c.seen[envelope.EventID] = envelope.Clone()
	if ReplayableEvent(envelope.Event) {
		c.checkpoint = envelope.EventID
	}
	return EventAcceptance{Applied: !ignored}, nil
}

func (c *Conversation) ignoreRecoveredOverlap(envelope RunEvent) (bool, error) {
	event := envelope.Event
	if delta, ok := event.(BlockDelta); ok {
		key := blockIdentity(envelope.RunID, delta.BlockID)
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
			return false, fmt.Errorf("%w: replayed start conflicts with the cold snapshot: %w", ErrEventConflict, err)
		}
		return true, nil
	case BlockDelta:
		key := blockIdentity(envelope.RunID, item.BlockID)
		_, exists := c.index[key]
		return !exists || !c.open[key], nil
	case BlockCompleted:
		key := blockIdentity(item.Block.RunID, item.Block.ID)
		at, exists := c.index[key]
		if !exists || c.open[key] {
			return false, nil
		}
		if !c.blocks[at].Equal(item.Block) {
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
	case strings.TrimSpace(envelope.StreamSegment()) == "":
		return errors.New("conversation: stream segment id is empty")
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
		if !started.Run.Lineage.IsRoot() && started.Run.Lineage.RootRunID != c.runID {
			return fmt.Errorf("conversation: child run %s belongs to root %s, not %s", envelope.RunID, started.Run.Lineage.RootRunID, c.runID)
		}
		if c.phase == ConversationWaiting && started.Run.Lineage.IsRoot() && c.runID != envelope.RunID {
			return fmt.Errorf("conversation: resumed root run %s does not match waiting run %s", envelope.RunID, c.runID)
		}
		return nil
	}
	run, exists := c.runs[envelope.RunID]
	if !exists {
		return fmt.Errorf("conversation: event references unknown run %s", envelope.RunID)
	}
	if run.Status != RunStatusRunning || run.ActiveSegmentID != envelope.SegmentID {
		return fmt.Errorf("conversation: event segment %s does not match active run %s segment %s", envelope.SegmentID, envelope.RunID, run.ActiveSegmentID)
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

func (c *Conversation) apply(envelope RunEvent) error {
	c.ensureStorage()
	var err error
	switch item := envelope.Event.(type) {
	case SegmentStarted:
		err = c.applySegmentStarted(item)
	case BlockStarted:
		err = c.applyBlockStarted(envelope.RunID, item)
	case BlockDelta:
		err = c.applyBlockDelta(envelope.RunID, item)
	case ToolArgumentsDelta:
		err = c.applyToolArgumentsDelta(envelope.RunID, item)
	case RunProgress:
		err = c.applyRunProgress(envelope.RunID, item)
	case CustomEvent:
		err = c.requireRunRunning(envelope.RunID, "publish a custom event")
	case BlockCompleted:
		err = c.applyBlockCompleted(envelope.RunID, item)
	case PlanChanged:
		err = c.applyPlanChanged(envelope.RunID, item)
	case RunInterrupted:
		err = c.applyInterrupted(envelope.RunID, item)
	case RunSuspended:
		err = c.applySuspended(envelope.RunID, item)
	case RunFinished:
		err = c.applyFinished(envelope.RunID, item)
	default:
		err = fmt.Errorf("conversation: event %T is unsupported", envelope.Event)
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
	run := event.Run
	previous, exists := c.runs[run.ID]
	if run.Lineage.IsRoot() {
		previousUsage := c.usage
		switch c.phase {
		case ConversationIdle:
			c.outcome = Outcome{}
			previousUsage = Usage{}
		case ConversationWaiting:
			if c.runID != run.ID {
				return fmt.Errorf("%w: cannot resume %s while %s is waiting", ErrInvalidTransition, run.ID, c.runID)
			}
		case ConversationRunning:
			if c.runID != "" && (!exists || previous.Status == RunStatusRunning) {
				return fmt.Errorf("%w: cannot start root segment while %s is running", ErrInvalidTransition, c.runID)
			}
		}
		if c.runID != "" && c.runID != run.ID {
			return fmt.Errorf("%w: root run changed from %s to %s", ErrInvalidTransition, c.runID, run.ID)
		}
		if err := validateUsageProgress(previousUsage, run.Usage); err != nil {
			return fmt.Errorf("%w: root segment started: %w", ErrInvalidTransition, err)
		}
		c.runID = run.ID
		c.phase = ConversationRunning
		c.interactions = nil
		c.usage = run.Usage.Clone()
	} else {
		if c.runID == "" || run.Lineage.RootRunID != c.runID {
			return fmt.Errorf("%w: child run %s has no active root", ErrInvalidTransition, run.ID)
		}
		if _, parentExists := c.runs[run.Lineage.ParentRunID]; !parentExists {
			return fmt.Errorf("%w: child run %s has unknown parent %s", ErrInvalidTransition, run.ID, run.Lineage.ParentRunID)
		}
		if exists && previous.Lineage != run.Lineage {
			return fmt.Errorf("%w: child run %s changed lineage", ErrInvalidTransition, run.ID)
		}
		if exists && previous.Status == RunStatusRunning {
			return fmt.Errorf("%w: child run %s started twice", ErrInvalidTransition, run.ID)
		}
		if exists {
			if err := validateUsageProgress(previous.Usage, run.Usage); err != nil {
				return fmt.Errorf("%w: child segment started: %w", ErrInvalidTransition, err)
			}
		}
	}
	c.runs[run.ID] = run.Clone()
	return nil
}

func (c *Conversation) applyBlockStarted(runID string, event BlockStarted) error {
	if err := c.requireRunRunning(runID, "start a block"); err != nil {
		return err
	}
	return c.put(event.Block, false)
}

func (c *Conversation) applyBlockDelta(runID string, event BlockDelta) error {
	if err := c.requireRunRunning(runID, "append a block delta"); err != nil {
		return err
	}
	key := blockIdentity(runID, event.BlockID)
	at, ok := c.index[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownBlock, event.BlockID)
	}
	if !c.open[key] {
		return fmt.Errorf("%w: block %s is already complete", ErrInvalidTransition, event.BlockID)
	}
	block := &c.blocks[at]
	switch block.Kind {
	case BlockAssistant:
		stream := c.textStreams[key]
		if _, err := stream.Apply(event); err != nil {
			return fmt.Errorf("%w: block %s: %w", ErrInvalidTransition, event.BlockID, err)
		}
		c.textStreams[key] = stream
		block.Text = stream.String()
	case BlockReasoning:
		if event.ContentIndex != nil {
			return fmt.Errorf("%w: reasoning block %s has a content index", ErrInvalidTransition, event.BlockID)
		}
		stream := c.textStreams[key]
		if _, err := stream.Apply(event); err != nil {
			return fmt.Errorf("%w: block %s: %w", ErrInvalidTransition, event.BlockID, err)
		}
		c.textStreams[key] = stream
		block.Text = stream.String()
	case BlockTool:
		if event.ContentIndex != nil {
			return fmt.Errorf("%w: tool block %s has a content index", ErrInvalidTransition, event.BlockID)
		}
		block.Tool.Output += event.Text
	default:
		return fmt.Errorf("%w: block %s of kind %s cannot stream", ErrInvalidTransition, event.BlockID, block.Kind)
	}
	return nil
}

func (c *Conversation) applyToolArgumentsDelta(runID string, event ToolArgumentsDelta) error {
	if err := c.requireRunRunning(runID, "append tool arguments"); err != nil {
		return err
	}
	key := blockIdentity(runID, event.BlockID)
	at, exists := c.index[key]
	if !exists {
		return fmt.Errorf("%w: %s", ErrUnknownBlock, event.BlockID)
	}
	block := c.blocks[at]
	if !c.open[key] || block.Kind != BlockTool {
		return fmt.Errorf("%w: block %s cannot stream tool arguments", ErrInvalidTransition, event.BlockID)
	}
	return nil
}

func (c *Conversation) applyRunProgress(runID string, event RunProgress) error {
	if err := c.requireRunRunning(runID, "report progress"); err != nil {
		return err
	}
	if event.Usage == nil {
		return nil
	}
	run := c.runs[runID]
	usage := event.Usage.Clone()
	usage.Steps, usage.Duration = run.Usage.Steps, run.Usage.Duration
	if usage.CostUSD == nil && run.Usage.CostUSD != nil {
		usage.CostUSD = new(*run.Usage.CostUSD)
	}
	if usage.ByModel == nil && run.Usage.ByModel != nil {
		usage.ByModel = run.Usage.Clone().ByModel
	}
	if err := validateUsageProgress(run.Usage, usage); err != nil {
		return fmt.Errorf("%w: run progress: %w", ErrInvalidTransition, err)
	}
	run.Usage = usage
	c.runs[runID] = run
	if runID == c.runID {
		c.usage = usage.Clone()
	}
	return nil
}

func (c *Conversation) applyBlockCompleted(runID string, event BlockCompleted) error {
	if err := c.requireRunRunning(runID, "complete a block"); err != nil {
		return err
	}
	return c.put(event.Block, true)
}

func (c *Conversation) applyPlanChanged(runID string, event PlanChanged) error {
	if runID != c.runID {
		return fmt.Errorf("%w: child run %s cannot change session plan", ErrInvalidTransition, runID)
	}
	if err := c.requireRunRunning(runID, "change the plan"); err != nil {
		return err
	}
	if event.Revision <= c.planRevision {
		return fmt.Errorf("%w: plan revision %d does not advance %d", ErrInvalidTransition, event.Revision, c.planRevision)
	}
	c.plan = slices.Clone(event.Items)
	c.planRevision = event.Revision
	return nil
}

func (c *Conversation) applyInterrupted(runID string, event RunInterrupted) error {
	if err := c.requireRunRunning(runID, "interrupt a run"); err != nil {
		return err
	}
	for _, interaction := range event.Interactions {
		itemID := InteractionItemID(interaction)
		at, exists := c.index[blockIdentity(runID, itemID)]
		if !exists {
			return fmt.Errorf("%w: interrupt references unknown item %s", ErrInvalidTransition, itemID)
		}
		if err := validateInteractionItem(interaction, c.blocks[at]); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidTransition, err)
		}
	}
	run := c.runs[runID]
	if err := validateUsageProgress(run.Usage, event.Usage); err != nil {
		return fmt.Errorf("%w: run interrupted: %w", ErrInvalidTransition, err)
	}
	pending := append(CloneInteractions(c.interactions), CloneInteractions(event.Interactions)...)
	if err := ValidateInteractions(pending); err != nil {
		return fmt.Errorf("%w: tree interrupt set: %v", ErrInvalidTransition, err)
	}
	run.Status = RunStatusWaiting
	run.ActiveSegmentID = ""
	run.Usage = event.Usage.Clone()
	c.runs[runID] = run
	if runID == c.runID {
		c.phase = ConversationWaiting
		c.usage = event.Usage.Clone()
	}
	c.reconciling = false
	c.coldTail = false
	c.interactions = pending
	return nil
}

func (c *Conversation) applySuspended(runID string, event RunSuspended) error {
	if err := c.requireRunRunning(runID, "suspend a run"); err != nil {
		return err
	}
	run := c.runs[runID]
	if err := validateUsageProgress(run.Usage, event.Usage); err != nil {
		return fmt.Errorf("%w: run suspended: %w", ErrInvalidTransition, err)
	}
	if runID == c.runID {
		if err := ValidateInteractions(c.interactions); err != nil {
			return fmt.Errorf("%w: root run suspended without a valid tree interrupt: %v", ErrInvalidTransition, err)
		}
	}
	run.Status = RunStatusWaiting
	run.ActiveSegmentID = ""
	run.Usage = event.Usage.Clone()
	c.runs[runID] = run
	if runID == c.runID {
		c.phase = ConversationWaiting
		c.usage = event.Usage.Clone()
		c.reconciling = false
		c.coldTail = false
	}
	return nil
}

func (c *Conversation) applyFinished(runID string, event RunFinished) error {
	run, exists := c.runs[runID]
	if !exists {
		return fmt.Errorf("%w: cannot finish unknown run %s", ErrInvalidTransition, runID)
	}
	if run.Status == RunStatusWaiting && event.Outcome.Status != OutcomeCanceled {
		return fmt.Errorf("%w: a waiting run can only finish by cancellation", ErrInvalidTransition)
	}
	if event.Outcome.Status == OutcomeCompleted && c.hasOpenBlocksForRun(runID) {
		return fmt.Errorf("%w: completed run %s still has open blocks", ErrInvalidTransition, runID)
	}
	if err := validateUsageProgress(run.Usage, event.Usage); err != nil {
		return fmt.Errorf("%w: run finished: %w", ErrInvalidTransition, err)
	}
	if runID == c.runID {
		for memberID, member := range c.runs {
			if memberID != runID && member.Lineage.RootRunID == runID && member.Status != RunStatusFinished {
				return fmt.Errorf("%w: root run finished while child %s is %s", ErrInvalidTransition, memberID, member.Status)
			}
		}
	}
	toolStatus := ToolError
	if event.Outcome.Status == OutcomeCanceled {
		toolStatus = ToolCanceled
	}
	c.settleOpenBlocksForRun(runID, toolStatus)
	run.Status = RunStatusFinished
	run.ActiveSegmentID = ""
	run.Outcome = event.Outcome.Clone()
	run.Usage = event.Usage.Clone()
	c.runs[runID] = run
	if runID == c.runID {
		c.phase = ConversationIdle
		c.reconciling = false
		c.coldTail = false
		c.interactions = nil
		c.outcome = event.Outcome.Clone()
		c.usage = event.Usage.Clone()
	}
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
	if !run.Lineage.IsRoot() {
		return errors.New("cannot settle conversation from a child-run control result")
	}
	if c.runID != "" && c.runID != run.ID {
		return fmt.Errorf("%w: settled run %s does not match %s", ErrInvalidTransition, run.ID, c.runID)
	}
	if c.runID == run.ID {
		if err := validateUsageProgress(c.usage, run.Usage); err != nil {
			return fmt.Errorf("%w: settled run: %w", ErrInvalidTransition, err)
		}
	}
	toolStatus := ToolError
	if run.Outcome.Status == OutcomeCanceled {
		toolStatus = ToolCanceled
	}
	c.settleOpenBlocks(toolStatus)
	for memberID, member := range c.runs {
		if member.Lineage.RootRunID != run.ID || member.Status == RunStatusFinished {
			continue
		}
		member.Status = RunStatusFinished
		member.ActiveSegmentID = ""
		member.Outcome = run.Outcome.Clone()
		c.runs[memberID] = member
	}
	c.runs[run.ID] = run.Clone()
	c.runID = run.ID
	c.segmentID = ""
	c.phase = ConversationIdle
	c.interactions = nil
	c.reconciling = false
	c.coldTail = false
	c.outcome = run.Outcome.Clone()
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
	c.textStreams = make(map[string]StreamedText)
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
		delete(c.textStreams, key)
		return nil
	}
	c.index[key] = len(c.blocks)
	c.blocks = append(c.blocks, block.Clone())
	c.open[key] = !completed
	if !completed && (block.Kind == BlockAssistant || block.Kind == BlockReasoning) {
		c.textStreams[key] = NewStreamedText(block.Text)
	}
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
	if c.runs == nil {
		c.runs = make(map[string]Run)
	}
	if c.open == nil {
		c.open = make(map[string]bool)
	}
	if c.textStreams == nil {
		c.textStreams = make(map[string]StreamedText)
	}
}

func (c *Conversation) rebuildBlockIndex() {
	c.index = make(map[string]int, len(c.blocks))
	c.open = make(map[string]bool, len(c.blocks))
	c.textStreams = make(map[string]StreamedText)
	for i, block := range c.blocks {
		key := blockIdentity(block.RunID, block.ID)
		c.index[key] = i
		c.open[key] = block.Status == BlockStatusRunning
		if block.Status == BlockStatusRunning && (block.Kind == BlockAssistant || block.Kind == BlockReasoning) {
			c.textStreams[key] = NewStreamedText(block.Text)
		}
	}
}

func (c *Conversation) hasOpenBlocksForRun(runID string) bool {
	for key, open := range c.open {
		if open && c.blocks[c.index[key]].RunID == runID {
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
		delete(c.textStreams, key)
	}
}

func (c *Conversation) settleOpenBlocksForRun(runID string, toolStatus ToolStatus) {
	for key, open := range c.open {
		if !open {
			continue
		}
		block := &c.blocks[c.index[key]]
		if block.RunID != runID {
			continue
		}
		if block.Kind == BlockTool && block.Tool != nil {
			block.Tool.Status = toolStatus
		}
		block.Status = BlockStatusIncomplete
		c.open[key] = false
		delete(c.textStreams, key)
	}
}

func blockIdentity(runID, blockID string) string { return runID + "\x00" + blockID }

func (c *Conversation) requireRunRunning(runID, action string) error {
	run, exists := c.runs[runID]
	if !exists || run.Status != RunStatusRunning {
		return fmt.Errorf("%w: cannot %s without active run %s", ErrInvalidTransition, action, runID)
	}
	return nil
}
