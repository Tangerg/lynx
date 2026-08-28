package agentexec

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	corechat "github.com/Tangerg/scope/core/chat"
)

var errInvalidModelContextCompaction = errors.New("agentexec: invalid model-context compaction")

// ModelContextTokenCalibration binds one provider-reported prompt footprint to
// the provider-neutral estimate of that exact request. The next request reuses
// only their delta, so provider framing/tokenizer behavior calibrates the same
// complete-request budget instead of creating a second token ledger.
type ModelContextTokenCalibration struct {
	reported  int64
	estimated int
}

func NewModelContextTokenCalibration(
	reported int64,
	estimated int,
) (ModelContextTokenCalibration, error) {
	if reported <= 0 {
		return ModelContextTokenCalibration{}, fmt.Errorf(
			"%w: reported context tokens must be positive",
			errInvalidModelContextCompaction,
		)
	}
	if estimated <= 0 {
		return ModelContextTokenCalibration{}, fmt.Errorf(
			"%w: estimated context tokens must be positive",
			errInvalidModelContextCompaction,
		)
	}
	return ModelContextTokenCalibration{reported: reported, estimated: estimated}, nil
}

func (m ModelContextTokenCalibration) Validate() error {
	if m == (ModelContextTokenCalibration{}) {
		return nil
	}
	if m.reported <= 0 || m.estimated <= 0 {
		return fmt.Errorf(
			"%w: token calibration requires positive reported and estimated values",
			errInvalidModelContextCompaction,
		)
	}
	return nil
}

func (m ModelContextTokenCalibration) ReportedTokens() int64 { return m.reported }
func (m ModelContextTokenCalibration) EstimatedTokens() int  { return m.estimated }

func (m ModelContextTokenCalibration) Adjustment() int {
	if m == (ModelContextTokenCalibration{}) {
		return 0
	}
	if m.reported > int64(math.MaxInt) {
		return math.MaxInt
	}
	return int(m.reported) - m.estimated
}

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
	calibration   ModelContextTokenCalibration
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
	calibration ModelContextTokenCalibration,
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
		calibration,
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
	calibration ModelContextTokenCalibration,
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
		calibration,
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
	calibration ModelContextTokenCalibration,
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
	if err := validateModelOutputReservation(selection, &options); err != nil {
		return ModelContextCompaction{}, fmt.Errorf(
			"%w: model output reservation: %w",
			errInvalidModelContextCompaction,
			err,
		)
	}
	if err := calibration.Validate(); err != nil {
		return ModelContextCompaction{}, err
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
		calibration:   calibration,
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

// Options returns the exact model-call options included in the complete request estimate.
func (m ModelContextCompaction) Options() corechat.Options { return m.options.Clone() }

// TokenEstimateAdjustment returns the provider calibration delta applied to
// every candidate built from this request's immutable model identity.
func (m ModelContextCompaction) TokenEstimateAdjustment() int {
	return m.calibration.Adjustment()
}

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
	messages        []corechat.Message
	changed         bool
	summarized      bool
	messagesBefore  int
	estimatedTokens int
}

// NewModelContextCompactionResult validates and freezes one effective suffix.
func NewModelContextCompactionResult(
	messages []corechat.Message,
	changed bool,
	summarized bool,
	messagesBefore int,
	estimatedTokens int,
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
	if estimatedTokens <= 0 {
		return ModelContextCompactionResult{}, fmt.Errorf(
			"%w: result token estimate must be positive",
			errInvalidModelContextCompaction,
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
		messages:        cloneChatMessages(messages),
		changed:         changed,
		summarized:      summarized,
		messagesBefore:  messagesBefore,
		estimatedTokens: estimatedTokens,
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

// EstimatedTokens returns the raw provider-neutral estimate for the exact
// request material returned by this reduction. The accounting owner pairs it
// with the provider's eventual input-token report.
func (m ModelContextCompactionResult) EstimatedTokens() int { return m.estimatedTokens }

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
