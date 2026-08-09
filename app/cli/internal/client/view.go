// Package client is the CLI's view of a lyra runtime, and the seam every
// command is written against.
//
// The types here are presentation models, deliberately not the runtime's wire
// contract. A terminal wants a flat, mutable, ordered transcript; the wire is
// shaped for replay, dedup and pagination. An implementation of [Runtime]
// translates one into the other — which is also what keeps a second copy of the
// wire types out of this module.
//
// Implementations: mock serves scripted conversations with no backend at all.
// The real ones — one embedding the runtime in-process, one attaching to an
// already-running server over HTTP — arrive when the runtime's protocol package
// moves out of its internal tree.
package client

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errors a [Runtime] reports by identity rather than by message, mirroring the
// symbolic names the runtime protocol uses for the same conditions. Commands
// branch on these; nothing branches on error text.
var (
	ErrSessionNotFound  = errors.New("session not found")
	ErrRunNotFound      = errors.New("run not found")
	ErrInterruptNotOpen = errors.New("interrupt not open")
	ErrSessionBusy      = errors.New("session has an active run")
	ErrRevisionConflict = errors.New("revision conflict")
	ErrEventGap         = errors.New("event gap")
	ErrEventConflict    = errors.New("event identity conflict")
	ErrDisconnected     = errors.New("runtime disconnected")
	ErrRequestConflict  = errors.New("idempotency request conflict")
)

// BlockKind names what a transcript block is. The set is closed: an item a
// [Runtime] implementation cannot classify becomes a [BlockNotice], never a new
// kind invented at the render site.
type BlockKind string

const (
	BlockUser      BlockKind = "user"
	BlockAssistant BlockKind = "assistant"
	BlockReasoning BlockKind = "reasoning"
	BlockTool      BlockKind = "tool"
	BlockNotice    BlockKind = "notice"
	BlockError     BlockKind = "error"
)

// Block is one renderable unit of a transcript, identified for the lifetime of
// its run so deltas can find it.
type Block struct {
	ID          string
	Kind        BlockKind
	Attachments []Attachment
	// Text is the block's body. Assistant and reasoning bodies are markdown and
	// arrive in pieces (see [BlockDelta]); the rest arrive whole.
	Text string
	// Tool carries the call's projection when Kind is [BlockTool].
	Tool *ToolCall
}

// ToolStatus is where a tool call is in its life.
type ToolStatus string

const (
	ToolRunning ToolStatus = "running"
	ToolOK      ToolStatus = "ok"
	ToolError   ToolStatus = "error"
)

// ToolKind is a terminal-relevant semantic category assigned by a runtime
// adapter. Delivery adapters switch on this closed projection, never on a
// provider's tool name.
type ToolKind string

const (
	ToolUnknown ToolKind = "unknown"
	ToolShell   ToolKind = "shell"
	ToolEdit    ToolKind = "edit"
	ToolRead    ToolKind = "read"
	ToolSearch  ToolKind = "search"
	ToolWeb     ToolKind = "web"
	ToolTask    ToolKind = "task"
)

// ToolCall is a tool invocation as the runtime adapter chose to present it.
//
// Every field is a projection the adapter already computed. Name preserves the
// provider-facing label for diagnostics; Kind and the structured fields below
// are the stable vocabulary renderers use. This keeps provider tool semantics in
// one adapter rather than rediscovering them in every UI.
type ToolCall struct {
	Kind    ToolKind
	Name    string
	Summary string
	Status  ToolStatus
	Command string
	Path    string
	Query   string
	URL     string
	Output  string
	// Diff is a unified diff when the call changed files.
	Diff string
	// ExitCode is set for completed process-like tools. Nil distinguishes an
	// absent code from a successful zero.
	ExitCode *int
	// Duration is how long the call took, once it has finished.
	Duration time.Duration
}

func (t ToolCall) Validate() error {
	var problems []error
	switch t.Kind {
	case ToolUnknown, ToolShell, ToolEdit, ToolRead, ToolSearch, ToolWeb, ToolTask:
	default:
		problems = append(problems, fmt.Errorf("kind %q is invalid", t.Kind))
	}
	switch t.Status {
	case ToolRunning, ToolOK, ToolError:
	default:
		problems = append(problems, fmt.Errorf("status %q is invalid", t.Status))
	}
	if t.Kind == ToolUnknown && strings.TrimSpace(t.Name) == "" {
		problems = append(problems, errors.New("unknown tool has no provider name"))
	}
	if t.Status == ToolRunning && t.ExitCode != nil {
		problems = append(problems, errors.New("running tool has an exit code"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("tool call: %w", err)
	}
	return nil
}

// PlanStatus is a plan item's state.
type PlanStatus string

const (
	PlanPending PlanStatus = "pending"
	PlanActive  PlanStatus = "active"
	PlanDone    PlanStatus = "done"
)

// PlanItem is one step of the run's plan.
type PlanItem struct {
	Title  string
	Status PlanStatus
}

// OutcomeStatus is how a run ended.
type OutcomeStatus string

const (
	OutcomeCompleted OutcomeStatus = "completed"
	OutcomeCanceled  OutcomeStatus = "canceled"
	OutcomeFailed    OutcomeStatus = "failed"
)

// Outcome is a finished run's verdict.
type Outcome struct {
	Status OutcomeStatus
	// Error carries the failure when Status is [OutcomeFailed].
	Error string
}
