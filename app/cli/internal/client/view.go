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

// ToolCall is a tool invocation as the runtime chose to present it.
//
// Every field is a projection the runtime already computed. The CLI renders
// Summary, Output and Diff as given and never reads Name to decide how: tool
// semantics belong to the runtime's toolset, and a renderer that special-cases
// a tool name becomes a second place those semantics live.
type ToolCall struct {
	Name    string
	Summary string
	Status  ToolStatus
	Output  string
	// Diff is a unified diff when the call changed files.
	Diff string
	// Duration is how long the call took, once it has finished.
	Duration time.Duration
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
