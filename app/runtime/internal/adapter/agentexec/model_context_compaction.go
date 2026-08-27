package agentexec

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	corechat "github.com/Tangerg/scope/core/chat"
)

var errInvalidModelContextCompaction = errors.New("agentexec: invalid model-context compaction")

type modelContextPersistence uint8

const (
	modelContextTransient modelContextPersistence = iota + 1
	modelContextDurable
)

// ModelContextCompaction is one immutable request to reduce only the mutable
// conversational portion of an Interaction model context. Instructions and
// newly applied input stay outside the foldable region and are reattached by
// the caller after the compactor returns.
type ModelContextCompaction struct {
	persistence   modelContextPersistence
	sessionID     string
	selection     modelref.Selection
	instructions  []corechat.Message
	candidate     []corechat.Message
	tools         []corechat.ToolDefinition
	options       corechat.Options
	protectedTail int
	preCompact    func(context.Context) bool
}

// NewDurableModelContextCompaction builds a request whose candidate begins
// with the Runtime's current durable conversation. The concrete compactor must
// prove that prefix against storage before it rewrites either representation.
func NewDurableModelContextCompaction(
	sessionID string,
	selection modelref.Selection,
	instructions []corechat.Message,
	candidate []corechat.Message,
	tools []corechat.ToolDefinition,
	options corechat.Options,
	protectedDurableTail int,
	preCompact func(context.Context) bool,
) (ModelContextCompaction, error) {
	return newModelContextCompaction(
		modelContextDurable,
		sessionID,
		selection,
		instructions,
		candidate,
		tools,
		options,
		protectedDurableTail,
		preCompact,
	)
}

// NewTransientModelContextCompaction builds a request for an isolated child
// Interaction whose complete candidate exists only in Strategy recovery state.
func NewTransientModelContextCompaction(
	sessionID string,
	selection modelref.Selection,
	instructions []corechat.Message,
	candidate []corechat.Message,
	tools []corechat.ToolDefinition,
	options corechat.Options,
	protectedTail int,
	preCompact func(context.Context) bool,
) (ModelContextCompaction, error) {
	return newModelContextCompaction(
		modelContextTransient,
		sessionID,
		selection,
		instructions,
		candidate,
		tools,
		options,
		protectedTail,
		preCompact,
	)
}

func newModelContextCompaction(
	persistence modelContextPersistence,
	sessionID string,
	selection modelref.Selection,
	instructions []corechat.Message,
	candidate []corechat.Message,
	tools []corechat.ToolDefinition,
	options corechat.Options,
	protectedTail int,
	preCompact func(context.Context) bool,
) (ModelContextCompaction, error) {
	if persistence != modelContextDurable && persistence != modelContextTransient {
		return ModelContextCompaction{}, errInvalidModelContextCompaction
	}
	if strings.TrimSpace(sessionID) == "" || sessionID != strings.TrimSpace(sessionID) {
		return ModelContextCompaction{}, fmt.Errorf(
			"%w: owning Session ID is required without surrounding whitespace",
			errInvalidModelContextCompaction,
		)
	}
	if err := selection.Validate(); err != nil {
		return ModelContextCompaction{}, fmt.Errorf(
			"%w: model selection: %w",
			errInvalidModelContextCompaction,
			err,
		)
	}
	if len(candidate) == 0 {
		return ModelContextCompaction{}, fmt.Errorf(
			"%w: candidate conversation is empty",
			errInvalidModelContextCompaction,
		)
	}
	if protectedTail < 0 || protectedTail > len(candidate) {
		return ModelContextCompaction{}, fmt.Errorf(
			"%w: protected tail %d is outside [0,%d]",
			errInvalidModelContextCompaction,
			protectedTail,
			len(candidate),
		)
	}
	for index := range instructions {
		if instructions[index].Role != corechat.RoleSystem {
			return ModelContextCompaction{}, fmt.Errorf(
				"%w: instruction %d is not a System message",
				errInvalidModelContextCompaction,
				index,
			)
		}
		if err := instructions[index].Validate(); err != nil {
			return ModelContextCompaction{}, fmt.Errorf(
				"%w: instruction %d: %w",
				errInvalidModelContextCompaction,
				index,
				err,
			)
		}
	}
	for index := range candidate {
		if err := candidate[index].Validate(); err != nil {
			return ModelContextCompaction{}, fmt.Errorf(
				"%w: candidate message %d: %w",
				errInvalidModelContextCompaction,
				index,
				err,
			)
		}
	}
	for index := range tools {
		if err := tools[index].Validate(); err != nil {
			return ModelContextCompaction{}, fmt.Errorf(
				"%w: Tool definition %d: %w",
				errInvalidModelContextCompaction,
				index,
				err,
			)
		}
	}
	if err := options.Validate(); err != nil {
		return ModelContextCompaction{}, fmt.Errorf(
			"%w: model options: %w",
			errInvalidModelContextCompaction,
			err,
		)
	}
	frozenTools := make([]corechat.ToolDefinition, len(tools))
	for index := range tools {
		frozenTools[index] = tools[index].Clone()
	}
	return ModelContextCompaction{
		persistence:   persistence,
		sessionID:     sessionID,
		selection:     selection,
		instructions:  cloneChatMessages(instructions),
		candidate:     cloneChatMessages(candidate),
		tools:         frozenTools,
		options:       options.Clone(),
		protectedTail: protectedTail,
		preCompact:    preCompact,
	}, nil
}

// Durable reports whether the foldable prefix must be reconciled with and
// atomically rewritten in the Runtime conversation store.
func (m ModelContextCompaction) Durable() bool {
	return m.persistence == modelContextDurable
}

// SessionID returns the Session that owns this root or delegated Interaction.
func (m ModelContextCompaction) SessionID() string { return m.sessionID }

// ModelSelection returns the model whose actual context limit governs this call.
func (m ModelContextCompaction) ModelSelection() modelref.Selection { return m.selection }

// Instructions returns an independently owned immutable prompt prefix.
func (m ModelContextCompaction) Instructions() []corechat.Message {
	return cloneChatMessages(m.instructions)
}

// Candidate returns the complete mutable suffix before reduction.
func (m ModelContextCompaction) Candidate() []corechat.Message {
	return cloneChatMessages(m.candidate)
}

// Tools returns the exact model-visible Tool manifest for budget estimation.
func (m ModelContextCompaction) Tools() []corechat.ToolDefinition {
	tools := make([]corechat.ToolDefinition, len(m.tools))
	for index := range m.tools {
		tools[index] = m.tools[index].Clone()
	}
	return tools
}

// Options returns the exact model-call options for fixed-overhead estimation.
func (m ModelContextCompaction) Options() corechat.Options { return m.options.Clone() }

// ProtectedTail returns how many candidate messages must remain verbatim.
func (m ModelContextCompaction) ProtectedTail() int { return m.protectedTail }

// AllowsCompaction applies the product lifecycle veto, when one is configured.
func (m ModelContextCompaction) AllowsCompaction(ctx context.Context) bool {
	return m.preCompact == nil || m.preCompact(ctx)
}

// ModelContextCompactionResult is the immutable effective mutable suffix after
// one compaction decision. Summarized distinguishes a semantic history fold
// from a coordinate-preserving deterministic trim.
type ModelContextCompactionResult struct {
	messages       []corechat.Message
	changed        bool
	summarized     bool
	messagesBefore int
}

// NewModelContextCompactionResult validates and freezes one effective suffix.
func NewModelContextCompactionResult(
	messages []corechat.Message,
	changed bool,
	summarized bool,
	messagesBefore int,
) (ModelContextCompactionResult, error) {
	if len(messages) == 0 {
		return ModelContextCompactionResult{}, fmt.Errorf(
			"%w: result messages are empty",
			errInvalidModelContextCompaction,
		)
	}
	if summarized && !changed {
		return ModelContextCompactionResult{}, fmt.Errorf(
			"%w: summarized result must be changed",
			errInvalidModelContextCompaction,
		)
	}
	if messagesBefore < 0 {
		return ModelContextCompactionResult{}, fmt.Errorf(
			"%w: result message count before compaction is negative: %d",
			errInvalidModelContextCompaction,
			messagesBefore,
		)
	}
	for index := range messages {
		if err := messages[index].Validate(); err != nil {
			return ModelContextCompactionResult{}, fmt.Errorf(
				"%w: result message %d: %w",
				errInvalidModelContextCompaction,
				index,
				err,
			)
		}
	}
	return ModelContextCompactionResult{
		messages:       cloneChatMessages(messages),
		changed:        changed,
		summarized:     summarized,
		messagesBefore: messagesBefore,
	}, nil
}

// Messages returns an independently owned effective mutable suffix.
func (m ModelContextCompactionResult) Messages() []corechat.Message {
	return cloneChatMessages(m.messages)
}

// Changed reports whether any candidate content was rewritten.
func (m ModelContextCompactionResult) Changed() bool { return m.changed }

// Summarized reports whether older messages were replaced by a semantic summary.
func (m ModelContextCompactionResult) Summarized() bool { return m.summarized }

// MessageCounts returns the before/after coordinates for observable boundaries.
func (m ModelContextCompactionResult) MessageCounts() (before int, after int) {
	return m.messagesBefore, len(m.messages)
}

// ModelContextCompactor owns the optional Runtime implementation of model-call
// context reduction. It may persist durable root history but never owns the
// Interaction state that consumes the returned replacement.
type ModelContextCompactor interface {
	// CompactModelContext returns the exact suffix the Interaction must install
	// before invoking the model.
	CompactModelContext(
		ctx context.Context,
		request ModelContextCompaction,
	) (ModelContextCompactionResult, error)
}
