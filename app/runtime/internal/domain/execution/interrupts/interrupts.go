// Package interrupts owns the durable hand-off between a running executor tree
// and a later continuation. Application orchestration records one root-owned
// [Pending] set when the tree reaches a human-input barrier; a resume consumes
// that complete set atomically.
package interrupts

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// Pending is one complete Run-tree barrier awaiting human decisions. The set is
// keyed by RootRunID and consumed all-or-nothing: individual source Runs do not
// own separate resume claims. Interrupts is the client-facing typed set;
// Suspensions binds each item back to the executor boundary it answers;
// Continuations is the application state required to reopen every surviving Run
// with a fresh Segment, including after process restart.
type Pending struct {
	RootRunID string
	SessionID string
	TurnID    string
	// GoalLeaseID is the root Run's autonomous-goal incarnation. It is an
	// application continuation fact, not executor payload: a resumed Segment
	// needs it to keep terminal budget accounting attached to the same Goal.
	GoalLeaseID   string
	Interrupts    []transcript.Interrupt
	Suspensions   []SuspensionBinding
	Continuations []Continuation
	// ProtocolProfile is the Run's frozen protocol contract, here for the same
	// reason and by the same guarantee as on the root Run. A continuation refuses
	// callers that cannot cover it and reuses its admitted interrupt kinds.
	ProtocolProfile execution.RunProtocolProfile
	// CreatedAt orders open sets. It is the barrier commit time, not any one
	// suspension's creation time.
	CreatedAt time.Time
}

// Continuation is the durable hand-off for one suspended Run. ProcessID is the
// opaque binding between that application Run and its executor member; the
// executor's parent/spawn topology remains inside its opaque checkpoint. Run
// lineage is the application's independent tree fact.
type Continuation struct {
	RunID          string
	ProcessID      string
	Lineage        execution.RunLineage
	ModelSelection modelref.Selection
	DrainedTools   []DrainedTool
	// CommittedTools are tool results whose transcript projection was completed
	// by an application transaction while the executor tree stayed parked. The
	// executor still publishes those results when it re-enters the checkpoint
	// because the model needs them in its continuation message; the resumed Run
	// reducer consumes this identity set without appending the Item a second time.
	CommittedTools []CommittedTool
	RunCreatedAt   time.Time
	Metrics        transcript.RunMetrics
	Limits         execution.RunLimits
}

// SuspensionBinding is the private correspondence between one client-visible
// interrupt item and the executor suspension that must receive its answer.
type SuspensionBinding struct {
	InterruptItemID string
	ProcessID       string
	SuspensionID    string
}

// SuspensionAnswer is one validated decision bound to the exact executor
// boundary that must consume it. InterruptItemID keeps the application item
// identity attached until the TurnControl adapter boundary; ProcessID and
// SuspensionID prevent execution from guessing which parked branch it answers.
type SuspensionAnswer struct {
	InterruptItemID string
	ProcessID       string
	SuspensionID    string
	Resolution      Resolution
}

// DrainedTool records one tool item that was still open when its Run suspended.
// The continuation re-binds the re-fired tool to this original item identity
// instead of minting a duplicate.
type DrainedTool struct {
	ItemID         string
	ItemOccurredAt time.Time
	CallID         string
	Name           string
	// Arguments is the canonical JSON used for resume correlation.
	Arguments string
}

// CommittedTool is the durable hand-off for one tool result already written to
// the transcript while its executor checkpoint was parked. Problem records the
// application classification that was committed; it is not reconstructed from
// the executor's lower-level error when the checkpoint later publishes its
// model-facing result.
type CommittedTool struct {
	ItemID string
	CallID string
	Name   string
	// Arguments is the canonical JSON used to reject a mismatched replay.
	Arguments string
	Problem   transcript.Problem
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

// ContinuationFor returns the hand-off for runID.
func (p Pending) ContinuationFor(runID string) (Continuation, bool) {
	for _, continuation := range p.Continuations {
		if continuation.RunID == runID {
			return continuation, true
		}
	}
	return Continuation{}, false
}

// Validate checks the complete tree hand-off. It deliberately validates both
// directions of the item/suspension relation so accepting a response never
// requires guessing which executor boundary it belongs to.
func (p Pending) Validate() error {
	switch {
	case strings.TrimSpace(p.RootRunID) == "":
		return errors.New("interrupts: pending root run id is required")
	case p.RootRunID != strings.TrimSpace(p.RootRunID):
		return errors.New("interrupts: pending root run id has surrounding whitespace")
	case strings.TrimSpace(p.SessionID) == "":
		return errors.New("interrupts: pending session id is required")
	case p.SessionID != strings.TrimSpace(p.SessionID):
		return errors.New("interrupts: pending session id has surrounding whitespace")
	case strings.TrimSpace(p.TurnID) == "":
		return errors.New("interrupts: pending turn id is required")
	case p.TurnID != strings.TrimSpace(p.TurnID):
		return errors.New("interrupts: pending turn id has surrounding whitespace")
	case p.GoalLeaseID != strings.TrimSpace(p.GoalLeaseID):
		return errors.New("interrupts: pending goal lease id has surrounding whitespace")
	case p.CreatedAt.IsZero():
		return errors.New("interrupts: pending creation time is required")
	case len(p.Interrupts) == 0:
		return errors.New("interrupts: pending set has no interrupts")
	case len(p.Continuations) == 0:
		return errors.New("interrupts: pending set has no continuations")
	case len(p.Suspensions) != len(p.Interrupts):
		return fmt.Errorf(
			"interrupts: %d suspension bindings do not match %d interrupts",
			len(p.Suspensions),
			len(p.Interrupts),
		)
	}
	if err := p.ProtocolProfile.Validate(); err != nil {
		return fmt.Errorf("interrupts: pending protocol profile: %w", err)
	}

	runIDs := make(map[string]struct{}, len(p.Continuations))
	processIDs := make(map[string]struct{}, len(p.Continuations))
	treeMembers := make([]execution.RunTreeMember, 0, len(p.Continuations))
	rootCount := 0
	for index, continuation := range p.Continuations {
		if err := continuation.Validate(); err != nil {
			return fmt.Errorf("interrupts: continuation[%d]: %w", index, err)
		}
		if _, duplicate := runIDs[continuation.RunID]; duplicate {
			return fmt.Errorf("interrupts: duplicate continuation run %q", continuation.RunID)
		}
		runIDs[continuation.RunID] = struct{}{}
		treeMembers = append(treeMembers, execution.RunTreeMember{
			RunID:   continuation.RunID,
			Lineage: continuation.Lineage,
		})
		if _, duplicate := processIDs[continuation.ProcessID]; duplicate {
			return fmt.Errorf("interrupts: duplicate continuation process %q", continuation.ProcessID)
		}
		processIDs[continuation.ProcessID] = struct{}{}
		if continuation.RunID == p.RootRunID {
			rootCount++
			if !continuation.Lineage.IsRoot() {
				return errors.New("interrupts: root continuation carries child lineage")
			}
		} else if continuation.Lineage.RootRunID != p.RootRunID {
			return fmt.Errorf(
				"interrupts: child continuation %q names root run %q, want %q",
				continuation.RunID,
				continuation.Lineage.RootRunID,
				p.RootRunID,
			)
		}
	}
	if rootCount != 1 {
		return fmt.Errorf("interrupts: pending set has %d root continuations", rootCount)
	}
	tree, err := execution.NewRunTree(p.RootRunID, treeMembers)
	if err != nil {
		return fmt.Errorf("interrupts: continuation tree: %w", err)
	}
	if len(p.Continuations) > 1 && !p.ProtocolProfile.ChildRuns {
		return errors.New("interrupts: pending tree has child Runs but its protocol profile forbids them")
	}
	canonicalRunIDs := tree.Postorder()
	for index, continuation := range p.Continuations {
		if continuation.RunID != canonicalRunIDs[index] {
			return fmt.Errorf(
				"interrupts: continuation[%d] is run %q, canonical postorder requires %q",
				index,
				continuation.RunID,
				canonicalRunIDs[index],
			)
		}
	}

	interruptsByItem := make(map[string]transcript.Interrupt, len(p.Interrupts))
	for index, interrupt := range p.Interrupts {
		if err := validateInterrupt(interrupt); err != nil {
			return fmt.Errorf("interrupts: interrupt[%d]: %w", index, err)
		}
		if _, exists := runIDs[interrupt.RunID]; !exists {
			return fmt.Errorf("interrupts: interrupt item %q names unknown run %q", interrupt.ItemID, interrupt.RunID)
		}
		if !slices.Contains(p.ProtocolProfile.InterruptKinds, interrupt.Kind) {
			return fmt.Errorf(
				"interrupts: interrupt item %q has kind %s outside the frozen protocol profile",
				interrupt.ItemID,
				interrupt.Kind,
			)
		}
		if _, duplicate := interruptsByItem[interrupt.ItemID]; duplicate {
			return fmt.Errorf("interrupts: duplicate interrupt item %q", interrupt.ItemID)
		}
		interruptsByItem[interrupt.ItemID] = interrupt
	}

	boundItems := make(map[string]struct{}, len(p.Suspensions))
	boundSuspensions := make(map[string]struct{}, len(p.Suspensions))
	for index, binding := range p.Suspensions {
		for _, identity := range []struct {
			name  string
			value string
		}{
			{name: "interrupt item id", value: binding.InterruptItemID},
			{name: "process id", value: binding.ProcessID},
			{name: "suspension id", value: binding.SuspensionID},
		} {
			if err := validateRequiredIdentity(identity.name, identity.value); err != nil {
				return fmt.Errorf("interrupts: suspension binding[%d]: %w", index, err)
			}
		}
		interrupt, exists := interruptsByItem[binding.InterruptItemID]
		if !exists {
			return fmt.Errorf(
				"interrupts: suspension binding[%d] names unknown item %q",
				index,
				binding.InterruptItemID,
			)
		}
		if p.Interrupts[index].ItemID != binding.InterruptItemID {
			return fmt.Errorf(
				"interrupts: suspension binding[%d] names item %q, canonical interrupt order requires %q",
				index,
				binding.InterruptItemID,
				p.Interrupts[index].ItemID,
			)
		}
		continuation, exists := continuationForProcess(p.Continuations, binding.ProcessID)
		if !exists {
			return fmt.Errorf(
				"interrupts: suspension binding[%d] names unknown process %q",
				index,
				binding.ProcessID,
			)
		}
		if continuation.RunID != interrupt.RunID {
			return fmt.Errorf(
				"interrupts: item %q belongs to run %q but its suspension belongs to run %q",
				interrupt.ItemID,
				interrupt.RunID,
				continuation.RunID,
			)
		}
		if _, duplicate := boundItems[binding.InterruptItemID]; duplicate {
			return fmt.Errorf("interrupts: item %q is bound more than once", binding.InterruptItemID)
		}
		boundItems[binding.InterruptItemID] = struct{}{}
		key := binding.ProcessID + "\x00" + binding.SuspensionID
		if _, duplicate := boundSuspensions[key]; duplicate {
			return fmt.Errorf(
				"interrupts: process %q suspension %q is bound more than once",
				binding.ProcessID,
				binding.SuspensionID,
			)
		}
		boundSuspensions[key] = struct{}{}
	}
	return nil
}

// Validate checks one Run-to-process continuation and all of its transcript
// hand-off identities independently of the root-owned Pending aggregate.
func (c Continuation) Validate() error {
	if err := validateRequiredIdentity("run id", c.RunID); err != nil {
		return err
	}
	if err := validateRequiredIdentity("process id", c.ProcessID); err != nil {
		return err
	}
	switch {
	case c.RunCreatedAt.IsZero():
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
		if err := tool.Problem.ValidateFor(transcript.ToolProblem); err != nil {
			return fmt.Errorf("committed tool[%d] problem: %w", index, err)
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

func validateInterrupt(interrupt transcript.Interrupt) error {
	if err := validateRequiredIdentity("item id", interrupt.ItemID); err != nil {
		return err
	}
	if interrupt.ItemOccurredAt.IsZero() {
		return errors.New("item occurrence time is required")
	}
	if err := validateRequiredIdentity("run id", interrupt.RunID); err != nil {
		return err
	}
	switch interrupt.Kind {
	case execution.ApprovalInterrupt:
		if interrupt.Approval == nil || interrupt.Question != nil {
			return errors.New("approval interrupt requires only an approval payload")
		}
	case execution.QuestionInterrupt:
		if interrupt.Question == nil || interrupt.Approval != nil {
			return errors.New("question interrupt requires only a question payload")
		}
	default:
		return fmt.Errorf("unknown interrupt kind %d", interrupt.Kind)
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

func continuationForProcess(continuations []Continuation, processID string) (Continuation, bool) {
	for _, continuation := range continuations {
		if continuation.ProcessID == processID {
			return continuation, true
		}
	}
	return Continuation{}, false
}
