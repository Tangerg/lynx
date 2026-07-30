// Package interrupts owns the durable hand-off between a running executor tree
// and a later continuation. Application orchestration records one root-owned
// [Pending] set when the tree reaches a human-input barrier; a resume consumes
// that complete set atomically.
package interrupts

import (
	"errors"
	"fmt"
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
	RootRunID     string
	SessionID     string
	TurnID        string
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

// Continuation is the durable hand-off for one suspended Run. Executor identity
// is recorded beside application identity because neither can be derived from
// the other after restart. ParentProcessID and SpawnCallID preserve the exact
// source envelope that future executor events must match.
type Continuation struct {
	RunID           string
	ProcessID       string
	ParentProcessID string
	SpawnCallID     string
	Lineage         execution.RunLineage
	ModelSelection  modelref.Selection
	DrainedTools    []DrainedTool
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
	ItemID string
	CallID string
	Name   string
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
	case strings.TrimSpace(p.SessionID) == "":
		return errors.New("interrupts: pending session id is required")
	case strings.TrimSpace(p.TurnID) == "":
		return errors.New("interrupts: pending turn id is required")
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

	runIDs := make(map[string]struct{}, len(p.Continuations))
	processIDs := make(map[string]struct{}, len(p.Continuations))
	continuationsByProcess := make(map[string]Continuation, len(p.Continuations))
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
		continuationsByProcess[continuation.ProcessID] = continuation
		if continuation.RunID == p.RootRunID {
			rootCount++
			if continuation.ParentProcessID != "" ||
				continuation.SpawnCallID != "" ||
				!continuation.Lineage.IsRoot() {
				return errors.New("interrupts: root continuation carries child lineage")
			}
		} else if continuation.ParentProcessID == "" || continuation.SpawnCallID == "" {
			return fmt.Errorf("interrupts: child continuation %q has incomplete process lineage", continuation.RunID)
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
	for _, continuation := range p.Continuations {
		if continuation.RunID == p.RootRunID {
			continue
		}
		parent, exists := continuationsByProcess[continuation.ParentProcessID]
		if !exists {
			return fmt.Errorf(
				"interrupts: child continuation %q names unknown parent process %q",
				continuation.RunID,
				continuation.ParentProcessID,
			)
		}
		if parent.RunID != continuation.Lineage.ParentRunID {
			return fmt.Errorf(
				"interrupts: child continuation %q process parent belongs to run %q, not lineage parent %q",
				continuation.RunID,
				parent.RunID,
				continuation.Lineage.ParentRunID,
			)
		}
	}
	tree, err := execution.NewRunTree(p.RootRunID, treeMembers)
	if err != nil {
		return fmt.Errorf("interrupts: continuation tree: %w", err)
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
		if _, duplicate := interruptsByItem[interrupt.ItemID]; duplicate {
			return fmt.Errorf("interrupts: duplicate interrupt item %q", interrupt.ItemID)
		}
		interruptsByItem[interrupt.ItemID] = interrupt
	}

	boundItems := make(map[string]struct{}, len(p.Suspensions))
	boundSuspensions := make(map[string]struct{}, len(p.Suspensions))
	for index, binding := range p.Suspensions {
		if strings.TrimSpace(binding.InterruptItemID) == "" ||
			strings.TrimSpace(binding.ProcessID) == "" ||
			strings.TrimSpace(binding.SuspensionID) == "" {
			return fmt.Errorf("interrupts: suspension binding[%d] is incomplete", index)
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
	switch {
	case strings.TrimSpace(c.RunID) == "":
		return errors.New("run id is required")
	case strings.TrimSpace(c.ProcessID) == "":
		return errors.New("process id is required")
	case c.ParentProcessID != strings.TrimSpace(c.ParentProcessID):
		return errors.New("parent process id has surrounding whitespace")
	case c.SpawnCallID != strings.TrimSpace(c.SpawnCallID):
		return errors.New("spawn call id has surrounding whitespace")
	case c.ParentProcessID == c.ProcessID:
		return errors.New("process cannot parent itself")
	case c.ParentProcessID == "" && c.SpawnCallID != "":
		return errors.New("root process carries a spawn call id")
	case c.RunCreatedAt.IsZero():
		return errors.New("run creation time is required")
	}
	if err := c.Lineage.Validate(c.RunID); err != nil {
		return fmt.Errorf("lineage: %w", err)
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
	switch {
	case strings.TrimSpace(itemID) == "":
		return errors.New("item id is required")
	case strings.TrimSpace(callID) == "":
		return errors.New("call id is required")
	case strings.TrimSpace(name) == "":
		return errors.New("name is required")
	case strings.TrimSpace(arguments) == "":
		return errors.New("arguments are required")
	default:
		return nil
	}
}

func validateInterrupt(interrupt transcript.Interrupt) error {
	switch {
	case strings.TrimSpace(interrupt.ItemID) == "":
		return errors.New("item id is required")
	case strings.TrimSpace(interrupt.RunID) == "":
		return errors.New("run id is required")
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

func continuationForProcess(continuations []Continuation, processID string) (Continuation, bool) {
	for _, continuation := range continuations {
		if continuation.ProcessID == processID {
			return continuation, true
		}
	}
	return Continuation{}, false
}
