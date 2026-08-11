package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// Pending is one complete Run-tree barrier awaiting human decisions. The set is
// keyed by RootRunID and consumed all-or-nothing: individual member Runs do not
// own separate resume claims. Interrupts is the published typed set;
// Bindings connects each item to the executor request it answers;
// Continuations is the durable state required to reopen every surviving Run
// with a fresh Segment, including after host restart.
type Pending struct {
	RootRunID  string
	SessionID  string
	ExecutorID string
	// GoalIncarnationID is the root Run's autonomous-goal incarnation. It is an
	// Run continuation fact, not executor payload: a resumed Segment
	// needs it to keep terminal budget accounting attached to the same Goal.
	GoalIncarnationID string
	Interrupts        []transcript.Interrupt
	Bindings          []InterruptBinding
	Continuations     []Continuation
	// Capabilities is the Run's frozen optional behavior. A continuation refuses
	// callers that lack it and reuses its admitted interrupt kinds.
	Capabilities run.Capabilities
	// CreatedAt orders open sets. It is the barrier commit time, not any one
	// input request's creation time.
	CreatedAt time.Time
}

// Continuation is the durable hand-off for one suspended Run. MemberID is the
// opaque binding between that Run and its executor member; the
// executor's parent/spawn topology remains inside its opaque checkpoint. Run
// lineage is the product's independent tree fact.
type Continuation struct {
	RunID          string
	MemberID       string
	Lineage        run.Lineage
	ModelSelection modelref.Selection
	DrainedTools   []DrainedTool
	// CommittedTools are tool results committed to the transcript while the
	// executor tree stayed parked. The
	// executor still publishes those results when it re-enters the checkpoint
	// because the model needs them in its continuation message; the resumed Run
	// reducer consumes this identity set without appending the Item a second time.
	CommittedTools []CommittedTool
	RunCreatedAt   time.Time
	Metrics        run.Metrics
	Limits         run.Limits
}

// InterruptBinding is the private correspondence between one published
// interrupt Item and the executor input request that must receive its answer.
type InterruptBinding struct {
	InterruptItemID string
	MemberID        string
	RequestID       string
}

// InterruptAnswer is one validated decision bound to the exact executor
// boundary that must consume it. InterruptItemID keeps the transcript item
// identity attached until the execution-control boundary; MemberID and
// RequestID prevents execution from guessing which parked branch it answers.
type InterruptAnswer struct {
	InterruptItemID string
	MemberID        string
	RequestID       string
	Resolution      interrupt.Resolution
}

// DrainedTool records one tool item that was still open when its Run suspended.
// The continuation re-binds the re-fired tool to this original item identity
// instead of minting a duplicate.
type DrainedTool struct {
	ItemID         string
	ItemOccurredAt time.Time
	CallID         string
	// SourceCallID is the provider ToolCall identity used by model-context results.
	SourceCallID string
	Name         string
	// Arguments is the canonical JSON used for resume correlation.
	Arguments string
}

// CommittedTool is the durable hand-off for one tool result already written to
// the transcript while its executor checkpoint was parked. Failure records the
// classification that was committed; it is not reconstructed from
// the executor's lower-level error when the checkpoint later publishes its
// model-facing result.
type CommittedTool struct {
	ItemID       string
	CallID       string
	SourceCallID string
	Name         string
	// Arguments is the canonical JSON used to reject a mismatched replay.
	Arguments string
	Failure   tool.Failure
}

// RootContinuation returns the root Run's hand-off. A valid Pending always has
// exactly one.
func (p Pending) RootContinuation() (Continuation, bool) {
	for _, continuation := range p.Continuations {
		if continuation.RunID == p.RootRunID {
			return continuation, true
		}
	}
	return Continuation{}, false
}

// Validate checks the complete tree hand-off. It deliberately validates both
// directions of the item/input-request relation so accepting a response never
// requires guessing which executor boundary it belongs to.
func (p Pending) Validate() error {
	if err := p.validateEnvelope(); err != nil {
		return err
	}
	if err := p.Capabilities.Validate(); err != nil {
		return fmt.Errorf("interrupts: pending capabilities: %w", err)
	}
	runIDs, err := p.validateContinuations()
	if err != nil {
		return err
	}
	interruptsByItem, err := p.validateInterrupts(runIDs)
	if err != nil {
		return err
	}
	return p.validateBindings(interruptsByItem)
}

func (p Pending) validateEnvelope() error {
	switch {
	case strings.TrimSpace(p.RootRunID) == "":
		return errors.New("interrupts: pending root run id is required")
	case p.RootRunID != strings.TrimSpace(p.RootRunID):
		return errors.New("interrupts: pending root run id has surrounding whitespace")
	case strings.TrimSpace(p.SessionID) == "":
		return errors.New("interrupts: pending session id is required")
	case p.SessionID != strings.TrimSpace(p.SessionID):
		return errors.New("interrupts: pending session id has surrounding whitespace")
	case strings.TrimSpace(p.ExecutorID) == "":
		return errors.New("interrupts: pending executor ID is required")
	case p.ExecutorID != strings.TrimSpace(p.ExecutorID):
		return errors.New("interrupts: pending executor ID has surrounding whitespace")
	case p.GoalIncarnationID != strings.TrimSpace(p.GoalIncarnationID):
		return errors.New("interrupts: pending goal incarnation id has surrounding whitespace")
	case p.CreatedAt.IsZero():
		return errors.New("interrupts: pending creation time is required")
	case len(p.Interrupts) == 0:
		return errors.New("interrupts: pending set has no interrupts")
	case len(p.Continuations) == 0:
		return errors.New("interrupts: pending set has no continuations")
	case len(p.Bindings) != len(p.Interrupts):
		return fmt.Errorf(
			"interrupts: %d input-request bindings do not match %d interrupts",
			len(p.Bindings),
			len(p.Interrupts),
		)
	}
	return nil
}

func (p Pending) validateContinuations() (map[string]struct{}, error) {
	runIDs := make(map[string]struct{}, len(p.Continuations))
	memberIDs := make(map[string]struct{}, len(p.Continuations))
	treeMembers := make([]run.TreeMember, 0, len(p.Continuations))
	rootCount := 0
	for index, continuation := range p.Continuations {
		if err := continuation.Validate(); err != nil {
			return nil, fmt.Errorf("interrupts: continuation[%d]: %w", index, err)
		}
		if _, duplicate := runIDs[continuation.RunID]; duplicate {
			return nil, fmt.Errorf("interrupts: duplicate continuation run %q", continuation.RunID)
		}
		runIDs[continuation.RunID] = struct{}{}
		treeMembers = append(treeMembers, run.TreeMember{
			RunID:   continuation.RunID,
			Lineage: continuation.Lineage,
		})
		if _, duplicate := memberIDs[continuation.MemberID]; duplicate {
			return nil, fmt.Errorf("interrupts: duplicate continuation member %q", continuation.MemberID)
		}
		memberIDs[continuation.MemberID] = struct{}{}
		if continuation.RunID == p.RootRunID {
			rootCount++
			if !continuation.Lineage.IsRoot() {
				return nil, errors.New("interrupts: root continuation carries child lineage")
			}
		} else if continuation.Lineage.RootRunID != p.RootRunID {
			return nil, fmt.Errorf(
				"interrupts: child continuation %q names root run %q, want %q",
				continuation.RunID,
				continuation.Lineage.RootRunID,
				p.RootRunID,
			)
		}
	}
	if rootCount != 1 {
		return nil, fmt.Errorf("interrupts: pending set has %d root continuations", rootCount)
	}
	tree, err := run.NewTree(p.RootRunID, treeMembers)
	if err != nil {
		return nil, fmt.Errorf("interrupts: continuation tree: %w", err)
	}
	if len(p.Continuations) > 1 && !p.Capabilities.ChildRuns {
		return nil, errors.New("interrupts: pending tree has child Runs but its capabilities forbid them")
	}
	canonicalRunIDs := tree.Postorder()
	for index, continuation := range p.Continuations {
		if continuation.RunID != canonicalRunIDs[index] {
			return nil, fmt.Errorf(
				"interrupts: continuation[%d] is run %q, canonical postorder requires %q",
				index,
				continuation.RunID,
				canonicalRunIDs[index],
			)
		}
	}
	return runIDs, nil
}

func (p Pending) validateInterrupts(runIDs map[string]struct{}) (map[string]transcript.Interrupt, error) {
	interruptsByItem := make(map[string]transcript.Interrupt, len(p.Interrupts))
	for index, interrupt := range p.Interrupts {
		if err := validateInterrupt(interrupt); err != nil {
			return nil, fmt.Errorf("interrupts: interrupt[%d]: %w", index, err)
		}
		if _, exists := runIDs[interrupt.RunID]; !exists {
			return nil, fmt.Errorf("interrupts: interrupt item %q names unknown run %q", interrupt.ItemID, interrupt.RunID)
		}
		if !slices.Contains(p.Capabilities.InterruptKinds, interrupt.Kind) {
			return nil, fmt.Errorf(
				"interrupts: interrupt item %q has kind %s outside the frozen capabilities",
				interrupt.ItemID,
				interrupt.Kind,
			)
		}
		if _, duplicate := interruptsByItem[interrupt.ItemID]; duplicate {
			return nil, fmt.Errorf("interrupts: duplicate interrupt item %q", interrupt.ItemID)
		}
		interruptsByItem[interrupt.ItemID] = interrupt
	}
	return interruptsByItem, nil
}

func (p Pending) validateBindings(interruptsByItem map[string]transcript.Interrupt) error {
	boundItems := make(map[string]struct{}, len(p.Bindings))
	boundRequests := make(map[string]struct{}, len(p.Bindings))
	for index, binding := range p.Bindings {
		for _, identity := range []struct {
			name  string
			value string
		}{
			{name: "interrupt item id", value: binding.InterruptItemID},
			{name: "member id", value: binding.MemberID},
			{name: "input request id", value: binding.RequestID},
		} {
			if err := validateRequiredIdentity(identity.name, identity.value); err != nil {
				return fmt.Errorf("interrupts: input-request binding[%d]: %w", index, err)
			}
		}
		interrupt, exists := interruptsByItem[binding.InterruptItemID]
		if !exists {
			return fmt.Errorf(
				"interrupts: input-request binding[%d] names unknown item %q",
				index,
				binding.InterruptItemID,
			)
		}
		if p.Interrupts[index].ItemID != binding.InterruptItemID {
			return fmt.Errorf(
				"interrupts: input-request binding[%d] names item %q, canonical interrupt order requires %q",
				index,
				binding.InterruptItemID,
				p.Interrupts[index].ItemID,
			)
		}
		continuation, exists := continuationForMember(p.Continuations, binding.MemberID)
		if !exists {
			return fmt.Errorf(
				"interrupts: input-request binding[%d] names unknown member %q",
				index,
				binding.MemberID,
			)
		}
		if continuation.RunID != interrupt.RunID {
			return fmt.Errorf(
				"interrupts: item %q belongs to run %q but its input request belongs to run %q",
				interrupt.ItemID,
				interrupt.RunID,
				continuation.RunID,
			)
		}
		if _, duplicate := boundItems[binding.InterruptItemID]; duplicate {
			return fmt.Errorf("interrupts: item %q is bound more than once", binding.InterruptItemID)
		}
		boundItems[binding.InterruptItemID] = struct{}{}
		key := binding.MemberID + "\x00" + binding.RequestID
		if _, duplicate := boundRequests[key]; duplicate {
			return fmt.Errorf(
				"interrupts: member %q input request %q is bound more than once",
				binding.MemberID,
				binding.RequestID,
			)
		}
		boundRequests[key] = struct{}{}
	}
	return nil
}

// Validate checks one Run-to-member continuation and all of its transcript
// hand-off identities independently of the root-owned Pending aggregate.
func (c Continuation) Validate() error {
	if err := validateRequiredIdentity("run id", c.RunID); err != nil {
		return err
	}
	if err := validateRequiredIdentity("member id", c.MemberID); err != nil {
		return err
	}
	if c.RunCreatedAt.IsZero() {
		return errors.New("run creation time is required")
	}
	if err := c.Lineage.Validate(c.RunID); err != nil {
		return fmt.Errorf("lineage: %w", err)
	}
	if err := c.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("model selection: %w", err)
	}
	if err := c.Metrics.Validate(); err != nil {
		return fmt.Errorf("metrics: %w", err)
	}
	if err := c.Limits.Validate(); err != nil {
		return fmt.Errorf("limits: %w", err)
	}
	openItems := make(map[string]struct{}, len(c.DrainedTools))
	openCalls := make(map[string]struct{}, len(c.DrainedTools))
	for index, tool := range c.DrainedTools {
		if err := validateToolIdentity(tool.ItemID, tool.CallID, tool.Name, tool.Arguments); err != nil {
			return fmt.Errorf("drained tool[%d]: %w", index, err)
		}
		if tool.SourceCallID != strings.TrimSpace(tool.SourceCallID) {
			return fmt.Errorf("drained tool[%d]: source call id has surrounding whitespace", index)
		}
		if tool.ItemOccurredAt.IsZero() {
			return fmt.Errorf("drained tool[%d]: item occurrence time is required", index)
		}
		if _, duplicate := openItems[tool.ItemID]; duplicate {
			return fmt.Errorf("drained tool item %q is duplicated", tool.ItemID)
		}
		if _, duplicate := openCalls[tool.CallID]; duplicate {
			return fmt.Errorf("drained tool call %q is duplicated", tool.CallID)
		}
		openItems[tool.ItemID] = struct{}{}
		openCalls[tool.CallID] = struct{}{}
	}
	committedItems := make(map[string]struct{}, len(c.CommittedTools))
	committedCalls := make(map[string]struct{}, len(c.CommittedTools))
	for index, tool := range c.CommittedTools {
		if err := validateToolIdentity(tool.ItemID, tool.CallID, tool.Name, tool.Arguments); err != nil {
			return fmt.Errorf("committed tool[%d]: %w", index, err)
		}
		if tool.SourceCallID != strings.TrimSpace(tool.SourceCallID) {
			return fmt.Errorf("committed tool[%d]: source call id has surrounding whitespace", index)
		}
		if err := tool.Failure.Validate(); err != nil {
			return fmt.Errorf("committed tool[%d] failure: %w", index, err)
		}
		if _, duplicate := committedItems[tool.ItemID]; duplicate {
			return fmt.Errorf("committed tool item %q is duplicated", tool.ItemID)
		}
		if _, duplicate := committedCalls[tool.CallID]; duplicate {
			return fmt.Errorf("committed tool call %q is duplicated", tool.CallID)
		}
		if _, open := openItems[tool.ItemID]; open {
			return fmt.Errorf("tool item %q is both drained and committed", tool.ItemID)
		}
		if _, open := openCalls[tool.CallID]; open {
			return fmt.Errorf("tool call %q is both drained and committed", tool.CallID)
		}
		committedItems[tool.ItemID] = struct{}{}
		committedCalls[tool.CallID] = struct{}{}
	}
	return nil
}

func validateToolIdentity(itemID, callID, name, arguments string) error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: "item id", value: itemID},
		{name: "call id", value: callID},
		{name: "name", value: name},
	} {
		if err := validateRequiredIdentity(identity.name, identity.value); err != nil {
			return err
		}
	}
	if strings.TrimSpace(arguments) == "" {
		return errors.New("arguments are required")
	}
	return nil
}

func validateInterrupt(request transcript.Interrupt) error {
	if err := validateRequiredIdentity("item id", request.ItemID); err != nil {
		return err
	}
	if request.ItemOccurredAt.IsZero() {
		return errors.New("item occurrence time is required")
	}
	if err := validateRequiredIdentity("run id", request.RunID); err != nil {
		return err
	}
	switch request.Kind {
	case interrupt.Approval:
		if request.Approval == nil || request.Question != nil {
			return errors.New("approval interrupt requires only an approval payload")
		}
		if err := request.Approval.Validate(); err != nil {
			return err
		}
	case interrupt.Question:
		if request.Question == nil || request.Approval != nil {
			return errors.New("question interrupt requires only a question payload")
		}
		if err := request.Question.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown interrupt kind %d", request.Kind)
	}
	return nil
}

func validateRequiredIdentity(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s has surrounding whitespace", name)
	}
	return nil
}

func continuationForMember(continuations []Continuation, memberID string) (Continuation, bool) {
	for _, continuation := range continuations {
		if continuation.MemberID == memberID {
			return continuation, true
		}
	}
	return Continuation{}, false
}
